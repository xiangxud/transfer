package client

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
)

func TestQueryUsesTableRESTAndPreservesIntegerTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != queryPath || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "root" || password != "secret" {
			t.Fatalf("unexpected credentials: %q %v", user, ok)
		}
		var payload Request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Database != "n9e" || payload.SQL != "SELECT time FROM metric" || payload.RowLimit != 250 {
			t.Fatalf("query payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"column_names":["time"],"data_types":["TIMESTAMP"],"values":[[1720000000123456]]}`)
	}))
	defer server.Close()

	c, err := New(Config{Address: server.URL, Username: "root", Password: "secret", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Query(context.Background(), "n9e", "SELECT time FROM metric", 250)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Values[0][0], json.Number("1720000000123456"); got != want {
		t.Fatalf("timestamp = %#v, want %#v", got, want)
	}
}

func TestInsertTabletSerializesRowsAndRejectsBadShape(t *testing.T) {
	var got Tablet
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != insertPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, `{"code":"200","message":"SUCCESS_STATUS"}`)
	}))
	defer server.Close()
	c, err := New(Config{Address: server.URL, Username: "root", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	payload := Tablet{
		Database: "n9e", Table: "m_0123456789abcdef",
		ColumnNames:      []string{"endpoint", "nid", "tagset", "value"},
		ColumnCategories: []string{"TAG", "TAG", "TAG", "FIELD"},
		DataTypes:        []string{"STRING", "STRING", "STRING", "DOUBLE"},
		Timestamps:       []int64{1720000000},
		Values:           [][]interface{}{{"host-1", "", `{"rack":"a"}`, 21.5}},
	}
	if err := c.InsertTablet(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, payload) {
		t.Fatalf("tablet = %+v, want %+v", got, payload)
	}

	payload.Values[0] = []interface{}{"host-1"}
	if err := c.InsertTablet(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "row 0") {
		t.Fatalf("malformed tablet error = %v", err)
	}
}

func TestNonQueryValidatesIoTDBStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":305,"message":"no available SchemaRegionGroups"}`)
	}))
	defer server.Close()
	c, err := New(Config{Address: server.URL, Username: "root", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.NonQuery(context.Background(), "n9e", "CREATE TABLE x"); err == nil ||
		!strings.Contains(err.Error(), "305") || !strings.Contains(err.Error(), "SchemaRegionGroups") {
		t.Fatalf("IoTDB error = %v", err)
	}
}

func TestClientRejectsInvalidConfigurationAndQueryLimit(t *testing.T) {
	for _, config := range []Config{
		{Address: "host:18080", Username: "root", Password: "secret"},
		{Address: "http://localhost:18080", Username: "root"},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("configuration %+v should fail", config)
		}
	}
	if ValidIdentifier("metrics_01") != true || ValidIdentifier("metrics; DROP DATABASE x") {
		t.Fatal("identifier validation mismatch")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("invalid row limit must not issue a request")
	}))
	defer server.Close()
	c, err := New(Config{Address: server.URL, Username: "root", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(context.Background(), "n9e", "SELECT 1", maxQueryLimit+1); err == nil {
		t.Fatal("oversized row limit should fail")
	}
}
