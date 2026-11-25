package iotdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend/iotdb/client"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/toolkits/pkg/logger"
)

const (
	seriesTable                  = "n9e_iotdb_series"
	defaultSendTaskSleepInterval = 50 * time.Millisecond
	defaultQueueMaxSize          = 102400
)

type Section struct {
	Enabled            bool   `yaml:"enabled"`
	Name               string `yaml:"name"`
	Address            string `yaml:"address"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	Database           string `yaml:"database"`
	TimestampPrecision string `yaml:"timestampPrecision"`
	Batch              int    `yaml:"batch"`
	WorkerNum          int    `yaml:"workerNum"`
	QueueSize          int    `yaml:"queueSize"`
	MaxRetry           int    `yaml:"maxRetry"`
	Timeout            int    `yaml:"timeout"`
	QueryRowLimit      int    `yaml:"queryRowLimit"`
}

type DataSource struct {
	Section Section
	client  *client.Client
	queue   chan dataobj.MetricValue

	precision int64
	tableMu   sync.Mutex
	tables    map[string]struct{}
	seriesMu  sync.Mutex
	series    map[string]struct{}
}

type metricGroup struct {
	metric string
	points []dataobj.MetricValue
}

type series struct {
	Metric   string
	Endpoint string
	Nid      string
	Tagset   string
	Tags     map[string]string
}

func (d *DataSource) Init() {
	if d.Section.Name == "" {
		d.Section.Name = "iotdb"
	}
	if d.Section.Database == "" {
		d.Section.Database = "n9e"
	}
	if !client.ValidIdentifier(d.Section.Database) {
		logger.Fatalf("invalid IoTDB database identifier %q", d.Section.Database)
	}
	if d.Section.Batch <= 0 {
		d.Section.Batch = 500
	}
	if d.Section.WorkerNum < 1 {
		d.Section.WorkerNum = 1
	}
	if d.Section.QueueSize < 1 {
		d.Section.QueueSize = defaultQueueMaxSize
	}
	if d.Section.MaxRetry < 1 {
		d.Section.MaxRetry = 3
	}
	if d.Section.Timeout < 1 {
		d.Section.Timeout = 5000
	}
	if d.Section.QueryRowLimit < 1 || d.Section.QueryRowLimit > 1000000 {
		d.Section.QueryRowLimit = 10000
	}
	switch d.Section.TimestampPrecision {
	case "", "ms":
		d.Section.TimestampPrecision = "ms"
		d.precision = 1000
	case "us":
		d.precision = 1000000
	case "ns":
		d.precision = 1000000000
	default:
		logger.Fatalf("IoTDB timestampPrecision must be ms, us, or ns")
	}

	var err error
	d.client, err = client.New(client.Config{
		Address:  d.Section.Address,
		Username: d.Section.Username,
		Password: d.Section.Password,
		Timeout:  time.Duration(d.Section.Timeout) * time.Millisecond,
	})
	if err != nil {
		logger.Fatalf("initialize IoTDB client: %v", err)
	}
	d.queue = make(chan dataobj.MetricValue, d.Section.QueueSize)
	d.tables = make(map[string]struct{})
	d.series = make(map[string]struct{})
	if err := d.ensureSchema(context.Background()); err != nil {
		logger.Fatalf("initialize IoTDB schema: %v", err)
	}
	for i := 0; i < d.Section.WorkerNum; i++ {
		go d.worker()
	}
}

func (d *DataSource) ensureSchema(ctx context.Context) error {
	if err := d.client.NonQuery(ctx, "", "CREATE DATABASE IF NOT EXISTS "+d.Section.Database); err != nil {
		return err
	}
	sql := "CREATE TABLE IF NOT EXISTS " + seriesTable + " (" +
		"time TIMESTAMP TIME, metric STRING TAG, endpoint STRING TAG, nid STRING TAG, " +
		"tagset STRING TAG, active BOOLEAN FIELD)"
	return d.client.NonQuery(ctx, d.Section.Database, sql)
}

func (d *DataSource) worker() {
	ticker := time.NewTicker(defaultSendTaskSleepInterval)
	defer ticker.Stop()
	batch := make([]dataobj.MetricValue, 0, d.Section.Batch)
	for {
		select {
		case point := <-d.queue:
			batch = append(batch, point)
			if len(batch) >= d.Section.Batch {
				d.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				d.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (d *DataSource) flush(points []dataobj.MetricValue) {
	groups := make(map[string]*metricGroup)
	for _, point := range points {
		if point.Metric == "" {
			logger.Warningf("skip IoTDB point without a metric name")
			continue
		}
		key := tableName(point.Metric)
		group := groups[key]
		if group == nil {
			group = &metricGroup{metric: point.Metric}
			groups[key] = group
		}
		group.points = append(group.points, point)
	}
	for _, group := range groups {
		var err error
		for attempt := 0; attempt < d.Section.MaxRetry; attempt++ {
			err = d.writeGroup(group)
			if err == nil {
				break
			}
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
		if err != nil {
			stats.Counter.Set("points.out.iotdb.err", len(group.points))
			logger.Errorf("write %d points for IoTDB metric %q: %v", len(group.points), group.metric, err)
		} else {
			stats.Counter.Set("points.out.iotdb", len(group.points))
		}
	}
}

func (d *DataSource) writeGroup(group *metricGroup) error {
	ctx := context.Background()
	table := tableName(group.metric)
	if err := d.ensureMetricTable(ctx, table); err != nil {
		return err
	}
	points, tags, err := coalesce(group.points)
	if err != nil {
		return err
	}
	if len(points) == 0 {
		return nil
	}
	timestamps := make([]int64, len(points))
	values := make([][]interface{}, len(points))
	for i, point := range points {
		timestamps[i] = point.Timestamp * d.precision
		value := interface{}(point.Value)
		if math.IsNaN(point.Value) || math.IsInf(point.Value, 0) {
			value = nil
		}
		values[i] = []interface{}{point.Endpoint, point.Nid, tags[i], value}
	}
	payload := client.Tablet{
		Database:         d.Section.Database,
		Table:            table,
		ColumnNames:      []string{"endpoint", "nid", "tagset", "value"},
		ColumnCategories: []string{"TAG", "TAG", "TAG", "FIELD"},
		DataTypes:        []string{"STRING", "STRING", "STRING", "DOUBLE"},
		Timestamps:       timestamps,
		Values:           values,
	}
	if err := d.client.InsertTablet(ctx, payload); err != nil {
		return err
	}
	return d.registerSeries(group.metric, points, tags)
}

func (d *DataSource) ensureMetricTable(ctx context.Context, table string) error {
	d.tableMu.Lock()
	defer d.tableMu.Unlock()
	if _, exists := d.tables[table]; exists {
		return nil
	}
	sql := "CREATE TABLE IF NOT EXISTS " + table + " (time TIMESTAMP TIME, endpoint STRING TAG, " +
		"nid STRING TAG, tagset STRING TAG, value DOUBLE FIELD)"
	if err := d.client.NonQuery(ctx, d.Section.Database, sql); err != nil {
		return err
	}
	d.tables[table] = struct{}{}
	return nil
}

func (d *DataSource) registerSeries(metric string, points []dataobj.MetricValue, tagsets []string) error {
	type key struct{ metric, endpoint, nid, tagset string }
	d.seriesMu.Lock()
	defer d.seriesMu.Unlock()
	keys := make(map[key]struct{})
	rows := make([][]interface{}, 0, len(points))
	for i, point := range points {
		item := key{metric: metric, endpoint: point.Endpoint, nid: point.Nid, tagset: tagsets[i]}
		encoded, _ := json.Marshal(item)
		cacheKey := string(encoded)
		if _, exists := d.series[cacheKey]; exists {
			continue
		}
		if _, exists := keys[item]; exists {
			continue
		}
		keys[item] = struct{}{}
		rows = append(rows, []interface{}{metric, point.Endpoint, point.Nid, tagsets[i], true})
	}
	if len(rows) == 0 {
		return nil
	}
	timestamps := make([]int64, len(rows))
	for i := range timestamps {
		timestamps[i] = 0
	}
	err := d.client.InsertTablet(context.Background(), client.Tablet{
		Database:         d.Section.Database,
		Table:            seriesTable,
		ColumnNames:      []string{"metric", "endpoint", "nid", "tagset", "active"},
		ColumnCategories: []string{"TAG", "TAG", "TAG", "TAG", "FIELD"},
		DataTypes:        []string{"STRING", "STRING", "STRING", "STRING", "BOOLEAN"},
		Timestamps:       timestamps,
		Values:           rows,
	})
	if err != nil {
		return err
	}
	for item := range keys {
		encoded, _ := json.Marshal(item)
		d.series[string(encoded)] = struct{}{}
	}
	return nil
}

func coalesce(points []dataobj.MetricValue) ([]dataobj.MetricValue, []string, error) {
	type key struct {
		endpoint string
		nid      string
		tagset   string
		ts       int64
	}
	positions := make(map[key]int, len(points))
	rows := make([]dataobj.MetricValue, 0, len(points))
	tagsets := make([]string, 0, len(points))
	for _, point := range points {
		encoded, err := tagset(point)
		if err != nil {
			return nil, nil, err
		}
		rowKey := key{endpoint: point.Endpoint, nid: point.Nid, tagset: encoded, ts: point.Timestamp}
		if index, exists := positions[rowKey]; exists {
			rows[index] = point
			continue
		}
		positions[rowKey] = len(rows)
		rows = append(rows, point)
		tagsets = append(tagsets, encoded)
	}
	return rows, tagsets, nil
}

func tagset(point dataobj.MetricValue) (string, error) {
	tags := point.TagsMap
	if len(tags) == 0 && point.Tags != "" {
		parsed, err := dataobj.SplitTagsString(point.Tags)
		if err != nil {
			return "", fmt.Errorf("parse tags for metric %q: %w", point.Metric, err)
		}
		tags = parsed
	}
	if tags == nil {
		tags = map[string]string{}
	}
	encoded, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("encode tags for metric %q: %w", point.Metric, err)
	}
	return string(encoded), nil
}

func tableName(metric string) string {
	hash := sha256.Sum256([]byte(metric))
	return "m_" + hex.EncodeToString(hash[:8])
}

func (d *DataSource) Push2Queue(items []*dataobj.MetricValue) {
	dropped := 0
	for _, item := range items {
		if item == nil || d.queue == nil {
			dropped++
			continue
		}
		copy := *item
		if item.TagsMap != nil {
			copy.TagsMap = make(map[string]string, len(item.TagsMap))
			for key, value := range item.TagsMap {
				copy.TagsMap[key] = value
			}
		}
		select {
		case d.queue <- copy:
		default:
			dropped++
		}
	}
	if dropped > 0 {
		stats.Counter.Set("iotdb.queue.err", dropped)
		logger.Errorf("dropped %d points because the IoTDB queue is full or not initialized", dropped)
	}
}

func (d *DataSource) GetInstance(metric, endpoint string, tags map[string]string) []string {
	if d.Section.Address == "" {
		return nil
	}
	return []string{d.Section.Address}
}
