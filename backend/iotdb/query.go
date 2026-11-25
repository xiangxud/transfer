package iotdb

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/calc"
	"github.com/toolkits/pkg/logger"
)

func (d *DataSource) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	responses := make([]*dataobj.TsdbQueryResponse, 0)
	for _, input := range inputs {
		for _, counter := range input.Counters {
			metric, wantTags := splitCounter(counter)
			if metric == "" {
				continue
			}
			if len(input.Nids) > 0 {
				for _, nid := range input.Nids {
					responses = append(responses, d.queryData(input, metric, counter, "", nid, wantTags))
				}
				continue
			}
			if len(input.Endpoints) > 0 {
				for _, endpoint := range input.Endpoints {
					responses = append(responses, d.queryData(input, metric, counter, endpoint, "", wantTags))
				}
				continue
			}
			responses = append(responses, d.queryData(input, metric, counter, "", "", wantTags))
		}
	}
	return responses
}

func (d *DataSource) queryData(input dataobj.QueryData, metric, counter, endpoint, nid string, wantTags map[string]string) *dataobj.TsdbQueryResponse {
	response := &dataobj.TsdbQueryResponse{
		Start: input.Start, End: input.End, Endpoint: endpoint, Nid: nid,
		Counter: counter, DsType: input.DsType, Step: input.Step,
		Values: make([]*dataobj.RRDData, 0),
	}
	ctx := context.Background()
	tagsets := []string{""}
	if len(wantTags) > 0 {
		seriesRows, err := d.querySeries(ctx, metric, endpointFilter(endpoint), nidFilter(nid))
		if err != nil {
			logger.Warningf("query IoTDB series for %s: %v", metric, err)
			return response
		}
		tagsets = tagsets[:0]
		for _, item := range seriesRows {
			if endpoint != "" && item.Endpoint != endpoint || nid != "" && item.Nid != nid {
				continue
			}
			if containsTags(item.Tags, wantTags) {
				tagsets = append(tagsets, item.Tagset)
			}
		}
		if len(tagsets) == 0 {
			return response
		}
	}
	for _, tagset := range tagsets {
		where := make([]string, 0, 4)
		if input.Start > 0 {
			where = append(where, fmt.Sprintf("time >= %d", input.Start*d.precision))
		}
		if input.End > 0 {
			where = append(where, fmt.Sprintf("time <= %d", input.End*d.precision))
		}
		if endpoint != "" {
			where = append(where, "endpoint="+quote(endpoint))
		}
		if nid != "" {
			where = append(where, "nid="+quote(nid))
		}
		if tagset != "" {
			where = append(where, "tagset="+quote(tagset))
		}
		sql := "SELECT time,value FROM " + tableName(metric)
		if len(where) > 0 {
			sql += " WHERE " + strings.Join(where, " AND ")
		}
		result, err := d.client.Query(ctx, d.Section.Database, sql, d.Section.QueryRowLimit)
		if err != nil {
			logger.Warningf("query IoTDB metric %s: %v", metric, err)
			continue
		}
		indices := columnIndices(result.ColumnNames)
		timeIndex, hasTime := indices["time"]
		valueIndex, hasValue := indices["value"]
		if !hasTime || !hasValue {
			logger.Warningf("IoTDB query for %s did not return time and value columns", metric)
			continue
		}
		for _, row := range result.Values {
			if len(row) <= timeIndex || len(row) <= valueIndex {
				continue
			}
			ts, ok := timestampSeconds(row[timeIndex], d.precision)
			value, valueOK := numericValue(row[valueIndex])
			if !ok || !valueOK || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			response.Values = append(response.Values, dataobj.NewRRDData(ts, value))
		}
	}
	sort.Slice(response.Values, func(i, j int) bool {
		return response.Values[i].Timestamp < response.Values[j].Timestamp
	})
	return response
}

func (d *DataSource) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
	tags := input.Tags
	if len(tags) == 0 {
		tags = []string{""}
	}
	counters := make([]string, 0, len(tags))
	for _, tag := range tags {
		counter, err := counterFor(input.Metric, tag)
		if err != nil {
			logger.Warningf("build IoTDB counter for %s: %v", input.Metric, err)
			continue
		}
		counters = append(counters, counter)
	}
	results := d.QueryData([]dataobj.QueryData{{
		Start: input.Start, End: input.End, Step: input.Step, DsType: input.DsType,
		ConsolFunc: input.ConsolFunc, Endpoints: input.Endpoints, Nids: input.Nids, Counters: counters,
	}})
	if input.AggrFunc == "" || len(results) < 2 {
		return results
	}
	groups := make(map[string][]*dataobj.TsdbQueryResponse)
	groupTags := make(map[string]string)
	for _, result := range results {
		tags, err := dataobj.SplitTagsString(counterTags(result.Counter))
		if err != nil {
			logger.Warningf("parse IoTDB response tags: %v", err)
			continue
		}
		if result.Nid != "" {
			tags["node"] = result.Nid
		} else if result.Endpoint != "" {
			tags["endpoint"] = result.Endpoint
		}
		selected := make(map[string]string)
		for _, key := range input.GroupKey {
			if value, exists := tags[key]; exists {
				selected[key] = value
			}
		}
		group := dataobj.SortedTags(selected)
		groups[group] = append(groups[group], result)
		groupTags[group] = group
	}
	aggregated := make([]*dataobj.TsdbQueryResponse, 0, len(groups))
	for key, values := range groups {
		counter := ""
		if groupTags[key] != "" {
			counter = "/" + input.AggrFunc + "," + groupTags[key]
		}
		aggregated = append(aggregated, &dataobj.TsdbQueryResponse{
			Start: input.Start, End: input.End, Counter: counter,
			Values: calc.Compute(input.AggrFunc, values),
		})
	}
	return aggregated
}

func (d *DataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	where := ""
	if len(recv.Endpoints) > 0 {
		where = " WHERE endpoint IN (" + quotedList(recv.Endpoints) + ")"
	}
	result, err := d.client.Query(context.Background(), d.Section.Database,
		"SELECT DISTINCT metric FROM "+seriesTable+where, d.Section.QueryRowLimit)
	if err != nil {
		logger.Warningf("query IoTDB metrics: %v", err)
		return nil
	}
	indices := columnIndices(result.ColumnNames)
	metricIndex, exists := indices["metric"]
	if !exists {
		return &dataobj.MetricResp{Metrics: []string{}}
	}
	metrics := make(map[string]struct{})
	for _, row := range result.Values {
		if len(row) > metricIndex {
			if value, ok := stringValue(row[metricIndex]); ok {
				metrics[value] = struct{}{}
			}
		}
	}
	resp := &dataobj.MetricResp{Metrics: make([]string, 0, len(metrics))}
	for metric := range metrics {
		resp.Metrics = append(resp.Metrics, metric)
	}
	sort.Strings(resp.Metrics)
	return resp
}

func (d *DataSource) QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	responses := make([]dataobj.IndexTagkvResp, 0, len(recv.Metrics))
	for _, metric := range recv.Metrics {
		seriesRows, err := d.querySeries(context.Background(), metric, recv.Endpoints)
		if err != nil {
			logger.Warningf("query IoTDB tags for %s: %v", metric, err)
			continue
		}
		values := make(map[string]map[string]struct{})
		for _, item := range seriesRows {
			if len(recv.Endpoints) > 0 && !contains(recv.Endpoints, item.Endpoint) {
				continue
			}
			for key, value := range item.Tags {
				if values[key] == nil {
					values[key] = make(map[string]struct{})
				}
				values[key][value] = struct{}{}
			}
		}
		pairs := make([]*dataobj.TagPair, 0, len(values))
		for key, set := range values {
			pair := &dataobj.TagPair{Key: key, Values: make([]string, 0, len(set))}
			for value := range set {
				pair.Values = append(pair.Values, value)
			}
			sort.Strings(pair.Values)
			pairs = append(pairs, pair)
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
		responses = append(responses, dataobj.IndexTagkvResp{Endpoints: recv.Endpoints, Metric: metric, Tagkv: pairs})
	}
	return responses
}

func (d *DataSource) QueryIndexByClude(recvs []dataobj.CludeRecv) []dataobj.XcludeResp {
	responses := make([]dataobj.XcludeResp, 0)
	for _, recv := range recvs {
		if len(recv.Endpoints) == 0 {
			continue
		}
		seriesRows, err := d.querySeries(context.Background(), recv.Metric, recv.Endpoints)
		if err != nil {
			logger.Warningf("query IoTDB index for %s: %v", recv.Metric, err)
			continue
		}
		byEndpoint := make(map[string]*dataobj.XcludeResp, len(recv.Endpoints))
		for _, endpoint := range recv.Endpoints {
			byEndpoint[endpoint] = &dataobj.XcludeResp{
				Endpoint: endpoint, Metric: recv.Metric, Tags: make([]string, 0), Step: 10, DsType: dataobj.GAUGE,
			}
		}
		for _, item := range seriesRows {
			response := byEndpoint[item.Endpoint]
			if response == nil || !matchesInclude(item.Tags, recv.Include) || matchesExclude(item.Tags, recv.Exclude) {
				continue
			}
			response.Tags = append(response.Tags, formatSeriesTags(item))
		}
		for _, response := range byEndpoint {
			sort.Strings(response.Tags)
			responses = append(responses, *response)
		}
	}
	return responses
}

func (d *DataSource) QueryIndexByFullTags(recvs []dataobj.IndexByFullTagsRecv) ([]dataobj.IndexByFullTagsResp, int) {
	responses := make([]dataobj.IndexByFullTagsResp, 0, len(recvs))
	for _, recv := range recvs {
		response := dataobj.IndexByFullTagsResp{
			Endpoints: recv.Endpoints, Metric: recv.Metric, Tags: make([]string, 0), Step: 10, DsType: dataobj.GAUGE,
		}
		if len(recv.Endpoints) > 0 {
			seriesRows, err := d.querySeries(context.Background(), recv.Metric, recv.Endpoints)
			if err != nil {
				logger.Warningf("query IoTDB full tags for %s: %v", recv.Metric, err)
			} else {
				for _, item := range seriesRows {
					response.Tags = append(response.Tags, formatSeriesTags(item))
				}
				sort.Strings(response.Tags)
			}
		}
		responses = append(responses, response)
	}
	return responses, len(responses)
}

func (d *DataSource) querySeries(ctx context.Context, metric string, endpoints []string, extraWhere ...string) ([]series, error) {
	where := []string{"metric=" + quote(metric)}
	if len(endpoints) > 0 {
		where = append(where, "endpoint IN ("+quotedList(endpoints)+")")
	}
	for _, clause := range extraWhere {
		if clause != "" {
			where = append(where, clause)
		}
	}
	sql := "SELECT DISTINCT metric,endpoint,nid,tagset FROM " + seriesTable + " WHERE " + strings.Join(where, " AND ")
	result, err := d.client.Query(ctx, d.Section.Database, sql, d.Section.QueryRowLimit)
	if err != nil {
		return nil, err
	}
	indices := columnIndices(result.ColumnNames)
	metricIndex, hasMetric := indices["metric"]
	endpointIndex, hasEndpoint := indices["endpoint"]
	nidIndex, hasNid := indices["nid"]
	tagsetIndex, hasTagset := indices["tagset"]
	if !hasMetric || !hasEndpoint || !hasNid || !hasTagset {
		return nil, fmt.Errorf("IoTDB series query returned unexpected columns %v", result.ColumnNames)
	}
	items := make([]series, 0, len(result.Values))
	for _, row := range result.Values {
		if len(row) <= tagsetIndex {
			continue
		}
		item := series{}
		item.Metric, _ = stringValue(row[metricIndex])
		item.Endpoint, _ = stringValue(row[endpointIndex])
		item.Nid, _ = stringValue(row[nidIndex])
		item.Tagset, _ = stringValue(row[tagsetIndex])
		item.Tags = parseTagset(item.Tagset)
		items = append(items, item)
	}
	return items, nil
}

func splitCounter(counter string) (string, map[string]string) {
	items := strings.SplitN(counter, "/", 2)
	if len(items) == 1 {
		return items[0], map[string]string{}
	}
	tags, err := dataobj.SplitTagsString(items[1])
	if err != nil {
		return items[0], map[string]string{}
	}
	return items[0], tags
}

func counterFor(metric, rawTags string) (string, error) {
	tags, err := dataobj.SplitTagsString(rawTags)
	if err != nil {
		return "", err
	}
	return dataobj.PKWithTags(metric, dataobj.SortedTags(tags)), nil
}

func counterTags(counter string) string {
	index := strings.Index(counter, "/")
	if index < 0 {
		return ""
	}
	return counter[index+1:]
}

func endpointFilter(endpoint string) []string {
	if endpoint == "" {
		return nil
	}
	return []string{endpoint}
}

func nidFilter(nid string) string {
	if nid == "" {
		return ""
	}
	return "nid=" + quote(nid)
}

func columnIndices(names []string) map[string]int {
	indices := make(map[string]int, len(names))
	for i, name := range names {
		indices[strings.ToLower(name)] = i
	}
	return indices
}

func timestampSeconds(value interface{}, precision int64) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		value, err := v.Int64()
		if err == nil {
			return value / precision, true
		}
	case float64:
		return int64(v) / precision, true
	case int64:
		return v / precision, true
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed / precision, true
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
			if parsed, err := time.ParseInLocation(layout, v, time.Local); err == nil {
				return parsed.Unix(), true
			}
		}
	}
	return 0, false
}

func numericValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func stringValue(value interface{}) (string, bool) {
	if value == nil {
		return "", false
	}
	if result, ok := value.(string); ok {
		return result, true
	}
	return fmt.Sprint(value), true
}

func parseTagset(encoded string) map[string]string {
	tags := make(map[string]string)
	if encoded == "" {
		return tags
	}
	if err := json.Unmarshal([]byte(encoded), &tags); err != nil {
		return map[string]string{}
	}
	return tags
}

func containsTags(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func matchesInclude(tags map[string]string, include []*dataobj.TagPair) bool {
	if len(include) == 0 || len(include) == 1 && include[0] == nil {
		return true
	}
	for _, pair := range include {
		if pair == nil {
			continue
		}
		for _, value := range pair.Values {
			if tags[pair.Key] == value {
				return true
			}
		}
	}
	return false
}

func matchesExclude(tags map[string]string, exclude []*dataobj.TagPair) bool {
	for _, pair := range exclude {
		if pair == nil {
			continue
		}
		for _, value := range pair.Values {
			if tags[pair.Key] == value {
				return true
			}
		}
	}
	return false
}

func formatSeriesTags(item series) string {
	tags := make(map[string]string, len(item.Tags)+1)
	for key, value := range item.Tags {
		tags[key] = value
	}
	if item.Endpoint != "" {
		tags["endpoint"] = item.Endpoint
	} else if item.Nid != "" {
		tags["nid"] = item.Nid
	}
	return dataobj.SortedTags(tags)
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func quotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quote(value))
	}
	return strings.Join(quoted, ",")
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
