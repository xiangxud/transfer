package iotdb

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend/iotdb/client"
)

func TestRemoteMetricAndSeries(t *testing.T) {
	address := os.Getenv("IOTDB_URL")
	if address == "" {
		t.Skip("set IOTDB_URL, IOTDB_USERNAME and IOTDB_PASSWORD to run against Table REST")
	}
	c, err := client.New(client.Config{Address: address, Username: os.Getenv("IOTDB_USERNAME"), Password: os.Getenv("IOTDB_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	database := fmt.Sprintf("n9e_smoke_%x", suffix)
	ctx := context.Background()
	if err := c.NonQuery(ctx, "", "CREATE DATABASE "+database); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.NonQuery(ctx, "", "DROP DATABASE "+database); err != nil {
			t.Errorf("drop %s: %v", database, err)
		} else {
			t.Logf("Dropped %s", database)
		}
	}()
	ds := &DataSource{Section: Section{Database: database, MaxRetry: 1, QueryRowLimit: 100}, client: c,
		precision: 1000, tables: make(map[string]struct{}), series: make(map[string]struct{})}
	if err := ds.ensureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	points := []dataobj.MetricValue{
		{Metric: "cpu.util", Endpoint: "host-1", Timestamp: 1720000000, Value: 12.5, TagsMap: map[string]string{"rack": "a"}},
		{Metric: "cpu.util", Endpoint: "host-1", Timestamp: 1720000001, Value: 13.75, TagsMap: map[string]string{"rack": "a"}},
	}
	if err := ds.writeGroup(&metricGroup{metric: "cpu.util", points: points}); err != nil {
		t.Fatal(err)
	}
	series, err := ds.querySeries(ctx, "cpu.util", []string{"host-1"})
	if err != nil || len(series) != 1 || series[0].Endpoint != "host-1" {
		t.Fatalf("series catalog = %+v, error = %v", series, err)
	}
	rows := ds.QueryData([]dataobj.QueryData{{Start: 1720000000, End: 1720000001, Step: 1,
		Counters: []string{"cpu.util/rack=a"}, Endpoints: []string{"host-1"}}})
	if len(rows) != 1 || len(rows[0].Values) != 2 {
		t.Fatalf("unexpected datasource result: %+v", rows)
	}
	for i, row := range rows[0].Values {
		if row.Timestamp != points[i].Timestamp || float64(row.Value) != points[i].Value {
			t.Fatalf("row %d: %+v", i, row)
		}
	}
}
