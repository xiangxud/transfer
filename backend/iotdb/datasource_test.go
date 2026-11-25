package iotdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend/iotdb/client"
)

func TestWriteGroupCreatesMetricTableAndSeriesCatalog(t *testing.T) {
	type request struct {
		path   string
		query  client.Request
		tablet client.Tablet
	}
	var requests []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var got request
		got.path = r.URL.Path
		switch r.URL.Path {
		case "/rest/table/v1/nonQuery", "/rest/table/v1/query":
			if err := json.NewDecoder(r.Body).Decode(&got.query); err != nil {
				t.Errorf("decode request: %v", err)
			}
		case "/rest/table/v1/insertTablet":
			if err := json.NewDecoder(r.Body).Decode(&got.tablet); err != nil {
				t.Errorf("decode tablet: %v", err)
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		requests = append(requests, got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"message":"SUCCESS_STATUS"}`)
	}))
	defer server.Close()

	iotdbClient, err := client.New(client.Config{
		Address: server.URL, Username: "root", Password: "secret", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ds := &DataSource{
		Section:   Section{Database: "n9e", MaxRetry: 1},
		client:    iotdbClient,
		precision: 1000,
		tables:    make(map[string]struct{}),
		series:    make(map[string]struct{}),
	}
	points := []dataobj.MetricValue{
		{Metric: "cpu.util", Endpoint: "host-1", Timestamp: 1720000000, Value: 10, TagsMap: map[string]string{"rack": "a"}},
		{Metric: "cpu.util", Endpoint: "host-1", Timestamp: 1720000001, Value: 20, TagsMap: map[string]string{"rack": "b"}},
	}
	if err := ds.writeGroup(&metricGroup{metric: "cpu.util", points: points}); err != nil {
		t.Fatalf("writeGroup() error = %v", err)
	}

	metricTable := tableName("cpu.util")
	if len(requests) != 3 {
		t.Fatalf("received %d REST calls, want create-table plus two tablets", len(requests))
	}
	wantCreateSQL := "CREATE TABLE IF NOT EXISTS " + metricTable + " (time TIMESTAMP TIME, endpoint STRING TAG, nid STRING TAG, tagset STRING TAG, value DOUBLE FIELD)"
	if got := requests[0]; got.path != "/rest/table/v1/nonQuery" || got.query.Database != "n9e" || got.query.SQL != wantCreateSQL {
		t.Fatalf("create-table request = %+v", got)
	}

	metricTablet := requests[1].tablet
	if requests[1].path != "/rest/table/v1/insertTablet" || metricTablet.Table != metricTable {
		t.Fatalf("metric write request = %+v", requests[1])
	}
	if !reflect.DeepEqual(metricTablet.ColumnNames, []string{"endpoint", "nid", "tagset", "value"}) ||
		!reflect.DeepEqual(metricTablet.ColumnCategories, []string{"TAG", "TAG", "TAG", "FIELD"}) ||
		!reflect.DeepEqual(metricTablet.DataTypes, []string{"STRING", "STRING", "STRING", "DOUBLE"}) {
		t.Fatalf("metric tablet schema = %+v", metricTablet)
	}
	if !reflect.DeepEqual(metricTablet.Timestamps, []int64{1720000000000, 1720000001000}) ||
		!reflect.DeepEqual(metricTablet.Values, [][]interface{}{
			{"host-1", "", `{"rack":"a"}`, float64(10)},
			{"host-1", "", `{"rack":"b"}`, float64(20)},
		}) {
		t.Fatalf("metric tablet data = %+v", metricTablet)
	}

	seriesTablet := requests[2].tablet
	if requests[2].path != "/rest/table/v1/insertTablet" || seriesTablet.Table != seriesTable {
		t.Fatalf("series catalog request = %+v", requests[2])
	}
	if !reflect.DeepEqual(seriesTablet.ColumnNames, []string{"metric", "endpoint", "nid", "tagset", "active"}) ||
		!reflect.DeepEqual(seriesTablet.Timestamps, []int64{0, 0}) {
		t.Fatalf("series catalog schema = %+v", seriesTablet)
	}
	gotTagsets := []string{seriesTablet.Values[0][3].(string), seriesTablet.Values[1][3].(string)}
	if !reflect.DeepEqual(gotTagsets, []string{`{"rack":"a"}`, `{"rack":"b"}`}) {
		t.Fatalf("series catalog tagsets = %v", gotTagsets)
	}
}

func TestQueryDataMapsIoTDBRowsToRRDValues(t *testing.T) {
	var query client.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/table/v1/query" {
			t.Errorf("path = %q, want table query endpoint", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Errorf("decode query: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"column_names":["time","value"],"data_types":["TIMESTAMP","DOUBLE"],"values":[[1720000000123,12.5],[1720000001123,13.75]]}`)
	}))
	defer server.Close()

	iotdbClient, err := client.New(client.Config{Address: server.URL, Username: "root", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ds := &DataSource{
		Section:   Section{Database: "n9e", QueryRowLimit: 100},
		client:    iotdbClient,
		precision: 1000,
	}
	input := dataobj.QueryData{Start: 0, End: 0, Step: 10, Counters: []string{"cpu.util"}, Endpoints: []string{"host-1"}}
	responses := ds.QueryData([]dataobj.QueryData{input})
	if len(responses) != 1 {
		t.Fatalf("response count = %d, want 1", len(responses))
	}
	response := responses[0]
	if response.Endpoint != "host-1" || response.Counter != "cpu.util" || len(response.Values) != 2 {
		t.Fatalf("response metadata/data = %+v", response)
	}
	if response.Values[0].Timestamp != 1720000000 || float64(response.Values[0].Value) != 12.5 ||
		response.Values[1].Timestamp != 1720000001 || float64(response.Values[1].Value) != 13.75 {
		t.Fatalf("mapped values = %+v", response.Values)
	}
	wantSQL := "SELECT time,value FROM " + tableName("cpu.util") + " WHERE endpoint='host-1'"
	if query.Database != "n9e" || query.SQL != wantSQL || query.RowLimit != 100 {
		t.Fatalf("query request = %+v, want database n9e, SQL %q and row limit 100", query, wantSQL)
	}
}

func TestQuoteEscapesIoTDBStringLiterals(t *testing.T) {
	if got := quote("host'1"); got != "'host''1'" {
		t.Fatalf("quote() = %q", got)
	}
	if strings.Contains(quotedList([]string{"a", "b'c"}), "b'c'") {
		t.Fatal("quoted list contains an unescaped quote")
	}
}

func TestEnsureSchemaCreatesDatabaseAndSeriesTable(t *testing.T) {
	var requests []client.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got client.Request
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, got)
		_, _ = io.WriteString(w, `{"code":200}`)
	}))
	defer server.Close()

	iotdbClient, err := client.New(client.Config{Address: server.URL, Username: "root", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	ds := &DataSource{Section: Section{Database: "n9e"}, client: iotdbClient}
	if err := ds.ensureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []client.Request{
		{SQL: "CREATE DATABASE IF NOT EXISTS n9e"},
		{Database: "n9e", SQL: "CREATE TABLE IF NOT EXISTS " + seriesTable + " (time TIMESTAMP TIME, metric STRING TAG, endpoint STRING TAG, nid STRING TAG, tagset STRING TAG, active BOOLEAN FIELD)"},
	}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("schema requests = %+v, want %+v", requests, want)
	}
}
