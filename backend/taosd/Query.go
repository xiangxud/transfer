package taosd

import (
	"fmt"
	"strings"
	"time"

	//"test/src/calc"
	//"test/src/dataobj"
	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/calc"
	"github.com/didi/nightingale/src/toolkits/stats"
	_ "github.com/taosdata/driver-go/taosSql"
	"github.com/toolkits/pkg/logger"
	//"github.com/didi/nightingale/src/toolkits/stats"
	//"github.com/toolkits/pkg/container/list"
	//"golang.org/x/sys/unix"
)

// query data for judge
func (taosd *TDengineDataSource) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data, inputs: %+v", inputs)
	//return nil
	workerNum := taosd.Section.config.queryworkers
	if workerNum <= 0 {
		workerNum = 100
	}
	datachannumber := taosd.Section.config.querydatachs
	if datachannumber <= 0 {
		datachannumber = 1000
	}

	worker := make(chan struct{}, workerNum) // 控制 goroutine 并发数
	dataChan := make(chan *dataobj.TsdbQueryResponse, datachannumber)

	done := make(chan struct{}, 1)
	resp := make([]*dataobj.TsdbQueryResponse, 0)
	go func() {
		defer func() { done <- struct{}{} }()
		for d := range dataChan {
			resp = append(resp, d)
		}
	}()

	for _, input := range inputs {
		if len(input.Nids) > 0 {
			for _, nid := range input.Nids {
				for _, counter := range input.Counters {
					worker <- struct{}{}
					go taosd.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
					//worker <- struct{}{}
				}
			}
		} else {
			for _, endpoint := range input.Endpoints {
				for _, counter := range input.Counters {
					worker <- struct{}{}
					go taosd.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
					//worker <- struct{}{}
				}
			}
		}

	}

	// 等待所有 goroutine 执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}
	close(dataChan)

	// 等待所有 dataChan 被消费完
	<-done

	return resp
}

// ConsolFunc 是RRD中的概念，比如：MIN|MAX|AVERAGE
func (taosd *TDengineDataSource) fetchDataSync(start, end int64, consolFun, nid, endpoint, counter string, step int, worker chan struct{}, dataChan chan *dataobj.TsdbQueryResponse) {
	defer func() {
		<-worker
	}()
	stats.Counter.Set("query.taosd", 1)

	if nid != "" {
		endpoint = dataobj.NidToEndpoint(nid)
	}

	data, err := taosd.fetchData(start, end, consolFun, endpoint, counter, step)
	if err != nil {
		logger.Warningf("fetch taosd data error: %+v", err)
		stats.Counter.Set("query.taosd.err", 1)
		data.Endpoint = endpoint
		data.Counter = counter
		data.Step = step
	}

	if nid != "" {
		data.Nid = nid
		data.Endpoint = ""
	} else {
		data.Endpoint = endpoint
	}

	dataChan <- data
}
func getTags(counter string) (tags string) {
	idx := strings.IndexAny(counter, "/")
	if idx == -1 {
		return ""
	}
	return counter[idx+1:]
}

func (taosd *TDengineDataSource) fetchData(start, end int64, consolFun, endpoint, counter string, step int) (*dataobj.TsdbQueryResponse, error) {
	var resp *dataobj.TsdbQueryResponse
	var metric string
	tags := make([]string, 0)
	if counter != "" {
		items := strings.SplitN(counter, "/", 2) //description:"metric/tags"
		metric = items[0]
		if len(items) > 1 {
			tags = strings.Split(items[1], ",")
			//tagMap := dataobj.DictedTagstring(items[1])
		}
	}
	c, err := NewInTaosdClient(taosd.Section)
	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil, err
	}
	defer c.Db.Close()
	endpoints := make([]string, 0)
	endpoints = append(endpoints, endpoint)

	taosdQuery := QueryData{
		Start:     start,
		End:       end,
		Metric:    metric,
		Endpoints: endpoints,
		Tags:      tags,
		Step:      step,
		//DsType:    input.DsType,
		GroupKey: []string{"ts"},
	}
	taosdQuery.renderSelect()
	taosdQuery.renderEndpoints()
	taosdQuery.renderTags()
	taosdQuery.renderTimeRange()
	//taosdQuery.renderGroupBy()

	logger.Debugf("query taosdql %s", taosdQuery.RawQuery)
	if taosd.Section.config.debugprt == 2 {
		fmt.Printf(taosdQuery.RawQuery)
	}
	//查询返回 按记录条->列名k 数据v保存为map数组
	resp, err = taosd.taosdQueryOne(c, taosdQuery)

	//qparm := genQParam(start, end, consolFun, endpoint, counter, step)
	//resp, err := tsdb.QueryOne(qparm)
	if err != nil {
		logger.Errorf("taosdQueryOne fail: %v", err)
		return resp, err
	}

	resp.Start = start
	resp.End = end

	return resp, nil
}
func GetCounter(metric, tag string, tagMap map[string]string) (counter string, err error) {
	if tagMap == nil {
		tagMap, err = dataobj.SplitTagsString(tag)
		if err != nil {
			logger.Errorf("split tag string error: %+v", err)
			return
		}
	}

	tagStr := dataobj.SortedTags(tagMap)
	counter = dataobj.PKWithTags(metric, tagStr)
	return
}

func (taosd *TDengineDataSource) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {

	logger.Debugf("query data for ui, input: %+v", input)

	workerNum := taosd.Section.config.queryworkers
	if workerNum <= 0 {
		workerNum = 100
	}
	datachannumber := taosd.Section.config.querydatachs
	if datachannumber <= 0 {
		datachannumber = 1000
	}
	worker := make(chan struct{}, workerNum) // 控制 goroutine 并发数
	dataChan := make(chan *dataobj.TsdbQueryResponse, datachannumber)

	done := make(chan struct{}, 1)
	resp := make([]*dataobj.TsdbQueryResponse, 0)
	go func() {
		defer func() { done <- struct{}{} }()
		for d := range dataChan {
			resp = append(resp, d)
		}
	}()

	if len(input.Nids) > 0 {
		for _, nid := range input.Nids {
			if len(input.Tags) == 0 {
				counter, err := GetCounter(input.Metric, "", nil)
				if err != nil {
					logger.Warningf("get counter error: %+v", err)
					continue
				}
				worker <- struct{}{}
				go taosd.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
				//worker <- struct{}{}
			} else {
				for _, tag := range input.Tags {
					counter, err := GetCounter(input.Metric, tag, nil)
					if err != nil {
						logger.Warningf("get counter error: %+v", err)
						continue
					}
					worker <- struct{}{}
					go taosd.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
					//worker <- struct{}{}
				}
			}
		}
	} else {
		for _, endpoint := range input.Endpoints {
			if len(input.Tags) == 0 {
				counter, err := GetCounter(input.Metric, "", nil)
				if err != nil {
					logger.Warningf("get counter error: %+v", err)
					continue
				}
				worker <- struct{}{}
				go taosd.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
				//worker <- struct{}{}
			} else {
				for _, tag := range input.Tags {
					counter, err := GetCounter(input.Metric, tag, nil)
					if err != nil {
						logger.Warningf("get counter error: %+v", err)
						continue
					}
					worker <- struct{}{}
					go taosd.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
					//worker <- struct{}{}
				}
			}
		}
	}

	// 等待所有 goroutine 执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}

	close(dataChan)
	<-done

	//进行数据计算
	aggrDatas := make([]*dataobj.TsdbQueryResponse, 0)
	if input.AggrFunc != "" && len(resp) > 1 {
		aggrCounter := make(map[string][]*dataobj.TsdbQueryResponse)

		// 没有聚合 tag, 或者曲线没有其他 tags, 直接所有曲线进行计算
		if len(input.GroupKey) == 0 || getTags(resp[0].Counter) == "" {
			aggrData := &dataobj.TsdbQueryResponse{
				Counter: input.AggrFunc,
				Start:   input.Start,
				End:     input.End,
				Values:  calc.Compute(input.AggrFunc, resp),
			}
			aggrDatas = append(aggrDatas, aggrData)
		} else {
			for _, data := range resp {
				counterMap := make(map[string]string)

				tagsMap, err := dataobj.SplitTagsString(getTags(data.Counter))
				if err != nil {
					logger.Warningf("split tag string error: %+v", err)
					continue
				}
				if data.Nid != "" {
					tagsMap["node"] = data.Nid
				} else {
					tagsMap["endpoint"] = data.Endpoint
				}

				// 校验 GroupKey 是否在 tags 中
				for _, key := range input.GroupKey {
					if value, exists := tagsMap[key]; exists {
						counterMap[key] = value
					}
				}

				counter := dataobj.SortedTags(counterMap)
				if _, exists := aggrCounter[counter]; exists {
					aggrCounter[counter] = append(aggrCounter[counter], data)
				} else {
					aggrCounter[counter] = []*dataobj.TsdbQueryResponse{data}
				}
			}

			// 有需要聚合的 tag 需要将 counter 带上
			for counter, datas := range aggrCounter {
				if counter != "" {
					counter = "/" + input.AggrFunc + "," + counter
				}
				aggrData := &dataobj.TsdbQueryResponse{
					Start:   input.Start,
					End:     input.End,
					Counter: counter,
					Values:  calc.Compute(input.AggrFunc, datas),
				}
				aggrDatas = append(aggrDatas, aggrData)
			}
		}
		return aggrDatas
	}
	return resp
}

//var QUERY_FROM_INDEX = true
type IndexMetricsResp struct {
	Data *dataobj.MetricResp `json:"dat"`
	Err  string              `json:"err"`
}

// query metrics & tags
func (taosd *TDengineDataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	/*
		if taosd.Section.config.useindex {
			var result IndexMetricsResp
			err := PostIndex("/api/index/metrics", int64(taosd.Section.config.callTimeout), recv, &result)
			if err != nil {
				logger.Errorf("post index failed, %+v", err)
				return nil
			}

			if result.Err != "" {
				logger.Errorf("index xclude failed, %+v", result.Err)
				return nil
			}

			return result.Data
		} else {
	*/
	logger.Debugf("query metric, recv: %+v", recv)
	c, err := NewInTaosdClient(taosd.Section)

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil
	}
	defer c.Db.Close()
	//use databasename;show stables;
	//metric as stable name,so query stables  name get metrics
	//	   taos> use log; show stables;
	//	   Database changed.

	//	                 name              |      created_time       | columns |  tags  |   tables    |
	//	   ============================================================================================
	//	    acct                           | 2020-11-19 12:31:55.449 |      22 |      1 |           1 |
	//	    dn                             | 2020-11-19 12:31:55.449 |      15 |      2 |           1 |
	//	   Query OK, 2 row(s) in set (0.002096s)

	taosdql := fmt.Sprintf("show stables;") //, taosd.Section.dbName)
	//查询返回 按记录条->列名k 数据v保存为map数组
	//通过查询超级表的名字，获取metrics的名字
	response := taosd.taosdSQL(c, taosdql)
	if response != nil {
		resp := &dataobj.MetricResp{
			Metrics: make([]string, 0),
		}
		for _, rowmap := range response {
			for colmmapk, colmmapv := range *rowmap {
				if colmmapk == "name" {
					stbname := colmmapv.(string)
					//转换间隔符'_'-->'.'
					metric := UnmetricnameEscape(stbname)
					resp.Metrics = append(resp.Metrics, metric)
				}
			}
		}
		return resp
	} else {
		if err != nil {
			logger.Warningf("query metrics on taosd error %v.", err)
		}
	}
	return nil
	//	}
}

/*
// query data for judge
func (taosd *TDengineDataSource) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data, inputs: %+v", inputs)

	c, err := NewInTaosdClient(taosd.Section)

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil
	}
	defer c.Db.Close()

	for _, input := range inputs {
		if len(input.Nids) > 0 {
			for _, nid := range input.Nids {
				for _, counter := range input.Counters {
					//worker <- struct{}{}
					//go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, nid, "", counter, input.Step, worker, dataChan)
				}
			}
		} else {
			for _, endpoint := range input.Endpoints {
				for _, counter := range input.Counters {
					//worker <- struct{}{}
					//go tsdb.fetchDataSync(input.Start, input.End, input.ConsolFunc, "", endpoint, counter, input.Step, worker, dataChan)
				}
			}
		}

	}






	respMap := make(map[string]*dataobj.TsdbQueryResponse)
	queryResponse := make([]*dataobj.TsdbQueryResponse, 0)
	for _, input := range inputs {
		for _, counter := range input.Counters {
			items := strings.SplitN(counter, "/", 2) //description:"metric/tags"
			metric := items[0]
			tags := make([]string, 0)
			if len(items) > 1 {
				tags = strings.Split(items[1], ",")
				tagMap := dataobj.DictedTagstring(items[1])
				if counter, err = dataobj.GetCounter(metric, "", tagMap); err != nil {
					logger.Warningf("get counter error: %+v", err)
					continue
				}
			}

			for _, endpoint := range input.Endpoints {
				key := fmt.Sprintf("%s%s", endpoint, counter)
				respMap[key] = &dataobj.TsdbQueryResponse{
					Start:    input.Start,
					End:      input.End,
					Endpoint: endpoint,
					Counter:  counter,
					DsType:   input.DsType,
					Step:     input.Step,
				}
			}

			taosdQuery := QueryData{
				Start:     input.Start,
				End:       input.End,
				Metric:    metric,
				Endpoints: input.Endpoints,
				Tags:      tags,
				Step:      input.Step,
				DsType:    input.DsType,
				GroupKey:  []string{"*"},
			}
			taosdQuery.renderSelect()
			taosdQuery.renderEndpoints()
			taosdQuery.renderTags()
			taosdQuery.renderTimeRange()
			taosdQuery.renderGroupBy()
			logger.Debugf("query taosdql %s", taosdQuery.RawQuery)

			//查询返回 按记录条->列名k 数据v保存为map数组
			response := taosd.taosdQuery(c, taosdQuery)

			if response != nil {
				m := taosd.divgroups(response)
				for _, li := range m {
					len := li.Len()
					Values := make([]*dataobj.RRDData, 0, len)
					for e := li.Front(); e != nil; e = e.Next() {
						Values = append(Values, e.pairdata)
						Tags := e.Tags
					}
					taosdCounter, err := dataobj.GetCounter(taosdQuery.Metric, "", Tags)
					if err != nil {
						logger.Warningf("get counter error: %+v", err)
						continue
					}
					//"endpointmetrick=v,k=v,k=v,k=v"
					key := fmt.Sprintf("%s%s", pairdata.endpoint, taosdCounter)
					if _, exists := respMap[key]; exists {
						respMap[key].Values = Values
					}

				}
			} else {
				if err != nil {
					logger.Warningf("query data point on taosd error %v.", err)
				} else if response.Error() != nil {
					logger.Warningf("query data point on taosd, resp error: %v.", response.Error())
				}
			}
		}
	}
	for _, resp := range respMap {
		queryResponse = append(queryResponse, resp)
	}

	return queryResponse
}
*/
// todo : 支持 comparison
// select value from metric where ...
/*
type QueryDataForUI struct {
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Metric      string   `json:"metric"`
	Endpoints   []string `json:"endpoints"`
	Nids        []string `json:"nids"`
	Tags        []string `json:"tags"`
	Step        int      `json:"step"`
	DsType      string   `json:"dstype"`
	GroupKey    []string `json:"groupKey"`                               //聚合维度
	AggrFunc    string   `json:"aggrFunc" description:"sum,avg,max,min"` //聚合计算
	ConsolFunc  string   `json:"consolFunc" description:"AVERAGE,MIN,MAX,LAST"`
	Comparisons []int64  `json:"comparisons"` //环比多少时间
}
{
    // 聚合(求和、均值、最大值、最小值)
    "aggrFunc": "",
    // 环比[0,7200,86400],单位秒
    "comparisons": [0],
    "consolFun": "AVERAGE",
    "dstype": "GUAGE",
    "end": 1606123412,
    // endpoint,可多主机对比
    "endpoints": [
        "192.168.2.44"
    ],
    "metric": "cpu.sys",
    "nids": null,
    "start": 1606119812,
    "step": 30,
    "tags": []
}
type QueryDataForUIResp struct {
	Start      int64      `json:"start"`
	End        int64      `json:"end"`
	Endpoint   string     `json:"endpoint"`
	Nid        string     `json:"nid"`
	Counter    string     `json:"counter"`
	DsType     string     `json:"dstype"`
	Step       int        `json:"step"`
	Values     []*RRDData `json:"values"`
	Comparison int64      `json:"comparison"`
}
// 返回数据
{
    // 返回的数据，每个对象对应请求参数中的不同参数，如多台主机、环比参数
    "dat": [
        {
            "comparison": 0,
            "counter": "cpu.sys",
            "dstype": "GAUGE",
            "end": 1606124849,
            "endpoint": "192.168.2.44",
            "nid": "",
            "start": 1606121249,
            "step": 30,
            "values": [
                {"timestamp": 1606121250, "value": 0.756303},
                {"timestamp": 1606121280, "value": 0.587741},
                {"timestamp": 1606121310, "value": 0.756303},
            ]
        },
        {
            "comparison": 7200,
            "counter": "cpu.sys",
            "dstype": "GAUGE",
            "end": 1606117649,
            "endpoint": "192.168.2.44",
            "nid": "",
            "start": 1606114049,
            "step": 30,
            "values": [
                {"timestamp": 1606121250, "value": 0.840336},
                {"timestamp": 1606121280, "value": 0.502934},
                {"timestamp": 1606121310, "value": 0.670578},
            ]
        }
    ],
    "err": ""
}

// query data for ui
func (taosd *TDengineDataSource) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {

	logger.Debugf("query data for ui, input: %+v", input)

	c, err := NewInTaosdClient(taosd.Section)
	defer c.Db.Close()

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil
	}

	taosdQuery := QueryData{
		Start:     input.Start,
		End:       input.End,
		Metric:    input.Metric,
		Endpoints: input.Endpoints,
		Tags:      input.Tags,
		Step:      input.Step,
		DsType:    input.DsType,
		GroupKey:  input.GroupKey,
		AggrFunc:  input.AggrFunc,
	}
	taosdQuery.renderSelect()
	taosdQuery.renderEndpoints()
	taosdQuery.renderTags()
	taosdQuery.renderTimeRange()
	taosdQuery.renderGroupBy()
	logger.Debugf("query taosdQuerysql %s", taosdQuery.RawQuery)

//////////
	   //select ts,value from stablename(taosdQuery.Metric) where endpoint=endpoint or
//	   taos> select last(*) from dn where dnodeid=1or fqdn="localhost:6030";
//	   last(ts)          |   last(cpu_taosd)    |   last(cpu_system)   | last(cpu_cores) |   last(mem_taosd)    |   last(mem_system)   | last(mem_total) |   last(disk_used)    | last(disk_total) |   last(band_speed)   |    last(io_read)     |    last(io_write)    | last(req_http) | last(req_select) | last(req_insert) |
//	   ===================================================================================================================================================================================================================================================================================================================================
//	   2020-11-24 10:38:30.879012 |              0.41012 |              5.67327 |               1 |             15.84375 |           1750.96484 |            1823 |             12.28959 |               26 |             21.44245 |              4.45801 |              7.49707 |              0 |                0 |                1 |
//	   Query OK, 1 row(s) in set (0.005490s)

///////////////////////

	//查询返回 按记录条->列名k 数据v保存为map数组
	response := taosd.taosdQuery(c, taosdQuery)

	if response != nil {
		m := taosd.divgroups(response)
		for endpoint, li := range m {

			len := li.Len()
			Values := make([]*dataobj.RRDData, 0, len)
			for e := li.Front(); e != nil; e = e.Next() {
				Values = append(Values, e.pairdata)
				Tags := e.Tags
			}

			taosdCounter, err := dataobj.GetCounter(taosdQuery.Metric, "", Tags)
			if err != nil {
				logger.Warningf("get counter error: %+v", err)
				continue
			}
			resp := &dataobj.TsdbQueryResponse{
				Start:    taosdQuery.Start,
				End:      taosdQuery.End,
				Endpoint: endpoint,
				Counter:  taosdCounter,
				DsType:   taosdQuery.DsType,
				Step:     taosdQuery.Step,
				Values:   Values,
			}
			queryResponse = append(queryResponse, resp)
		}
	}
	return queryResponse
}

// query metrics & tags
func (taosd *TDengineDataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	logger.Debugf("query metric, recv: %+v", recv)
	c, err := NewInTaosdClient(taosd.Section)
	defer c.Db.Close()

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil
	}

	   //use databasename;show stables;
	   //metric as stable name,so query stables  name get metrics
//	   taos> use log; show stables;
//	   Database changed.

//	                 name              |      created_time       | columns |  tags  |   tables    |
//	   ============================================================================================
//	    acct                           | 2020-11-19 12:31:55.449 |      22 |      1 |           1 |
//	    dn                             | 2020-11-19 12:31:55.449 |      15 |      2 |           1 |
//	   Query OK, 2 row(s) in set (0.002096s)

	taosdql := fmt.Sprintf("show stables;") //, taosd.Section.dbName)
	//查询返回 按记录条->列名k 数据v保存为map数组
	//通过查询超级表的名字，获取metrics的名字
	response := taosd.taosdSQL(c, taosdql)
	if response != nil {
		resp := &dataobj.MetricResp{
			Metrics: make([]string, 0),
		}
		for _, rowmap := range response {
			for colmmapk, colmmapv := range rowmap {
				if colmmapk == "name" {
					stbname := colmmapv
					//转换间隔符'_'-->'.'
					metrics := unmetricnameEscape(stbname)
					resp.Metrics = append(resp.Metrics, metric)
				}
			}
		}
		return resp
	} else {
		if err != nil {
			logger.Warningf("query metrics on taosd error %v.", err)
		} else if response.Error() != nil {
			logger.Warningf("query metrics on taosd, resp error: %v.", response.Error())
		}
	}
	return nil
}
*/
type IndexTagPairsResp struct {
	Data []dataobj.IndexTagkvResp `json:"dat"`
	Err  string                   `json:"err"`
}

func (taosd *TDengineDataSource) QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	/*	if taosd.Section.config.useindex {
			var result IndexTagPairsResp
			err := PostIndex("/api/index/tagkv", int64(taosd.Section.config.callTimeout), recv, &result)
			if err != nil {
				logger.Errorf("post index failed, %+v", err)
				return nil
			}

			if result.Err != "" || len(result.Data) == 0 {
				logger.Errorf("index xclude failed, %+v", result.Err)
				return nil
			}

			return result.Data
		} else {
	*/
	logger.Debugf("query tag pairs, recv: %+v", recv)

	c, err := NewInTaosdClient(taosd.Section)

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil
	}
	defer c.Db.Close()
	resp := make([]dataobj.IndexTagkvResp, 0)
	for _, metric := range recv.Metrics {
		tagkvResp := dataobj.IndexTagkvResp{
			Endpoints: recv.Endpoints,
			Metric:    metric,
			Tagkv:     make([]*dataobj.TagPair, 0),
		}
		tagkvResp.Tagkv = taosd.getTagMaps(c, metric, taosd.Section.config.dbName, recv.Endpoints)

		resp = append(resp, tagkvResp)
	}

	return resp
	//	}
}

/*
/////////////////////////////////////
taos> DESCRIBE dn;
             Field              |        Type        |   Length    |    Note    |
=================================================================================
 ts                             | TIMESTAMP          |           8 |            |
 cpu_taosd                      | FLOAT              |           4 |            |
 cpu_system                     | FLOAT              |           4 |            |
 cpu_cores                      | INT                |           4 |            |
 mem_taosd                      | FLOAT              |           4 |            |
 mem_system                     | FLOAT              |           4 |            |
 mem_total                      | INT                |           4 |            |
 disk_used                      | FLOAT              |           4 |            |
 disk_total                     | INT                |           4 |            |
 band_speed                     | FLOAT              |           4 |            |
 io_read                        | FLOAT              |           4 |            |
 io_write                       | FLOAT              |           4 |            |
 req_http                       | INT                |           4 |            |
 req_select                     | INT                |           4 |            |
 req_insert                     | INT                |           4 |            |
 dnodeid                        | INT                |           4 | TAG        |
 fqdn                           | BINARY             |         128 | TAG        |
Query OK, 17 row(s) in set (0.000378s)
*/
///////////////////////////////
type IndexCludeResp struct {
	Data []dataobj.XcludeResp `json:"dat"`
	Err  string               `json:"err"`
}

func (taosd *TDengineDataSource) QueryIndexByClude(recvs []dataobj.CludeRecv) []dataobj.XcludeResp {
	/*	if taosd.Section.config.useindex {
			var result IndexCludeResp
			err := PostIndex("/api/index/counter/clude", int64(taosd.Section.config.callTimeout), recvs, &result)
			if err != nil {
				logger.Errorf("post index failed, %+v", err)
				return nil
			}

			if result.Err != "" || len(result.Data) == 0 {
				logger.Errorf("index xclude failed, %+v", result.Err)
				return nil
			}

			return result.Data
		} else {
	*/
	logger.Debugf("query IndexByClude , recv: %+v", recvs)

	c, err := NewInTaosdClient(taosd.Section)

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil
	}
	defer c.Db.Close()
	resps := make([]dataobj.XcludeResp, 0)

	for _, recv := range recvs {

		if len(recv.Endpoints) == 0 {
			continue
		}
		//获取超级表结构，表名以metric创建的
		stabname := MetricnameEscape(recv.Metric)
		//通过表结构获取TAGS字段名数组
		tagsMap := taosd.taosdGetTagsDescribe(c, stabname)
		xcludeRespMap := make(map[string]*dataobj.XcludeResp)
		for _, endpoint := range recv.Endpoints {
			key := fmt.Sprintf("endpoint=%s", endpoint)
			xcludeRespMap[key] = &dataobj.XcludeResp{
				Endpoint: endpoint,
				Metric:   recv.Metric,
				Tags:     make([]string, 0),
				Step:     10,
				DsType:   "GAUGE",
			}
		}

		taosdql := "select endpoint , "
		for _, tagname := range tagsMap {
			taosdql = taosdql + fmt.Sprintf(" %s,", tagname)
		}
		taosdql = taosdql[:len(taosdql)-len(",")]

		taosdShow := ShowSeries{
			Database:  c.Database,
			Metric:    stabname,
			Endpoints: recv.Endpoints,
			Start:     time.Now().AddDate(0, 0, -30).Unix(),
			End:       time.Now().Unix(),
			Include:   recv.Include,
			Exclude:   recv.Exclude,
		}

		taosdShow.renderEndpoints()
		taosdShow.renderInclude()
		taosdShow.renderExclude()

		taosdql = fmt.Sprintf("%s from  %s  %s;", taosdql, stabname, taosdShow.RawQuery)
		response := taosd.taosdSQL(c, taosdql)
		if response != nil {
			//resp := &dataobj.MetricResp{
			//	Metrics: make([]string, 0),
			//}

			for _, rowmap := range response {
				var curendpoint string
				//var curItem string
				tags := make(map[string]string, 0)
				for tagkey, tagValue := range *rowmap {
					if tagkey == "endpoint" {
						curendpoint = tagValue.(string)
						continue
					}
					if tagValue == nil {
						continue
					}
					tags[tagkey] = tagValue.(string)
					//tag := tagkey + "=" + tagValue.(string)
					//tags = append(tags, tag)

				}
				if tags != nil {
					curItem := fmt.Sprintf("endpoint=%s", curendpoint)
					//tags = tags[:len(tags)-len(",")]
					xcludeRespMap[curItem].Tags = append(xcludeRespMap[curItem].Tags, dataobj.SortedTags(tags))
					xcludeRespMap[curItem].Step = 10
				}
			}
		} else {
			if err != nil {
				logger.Warningf("query index by clude on taosd error: %v.", err)
			}
		}
		for _, xcludeResp := range xcludeRespMap {
			resps = append(resps, *xcludeResp)
		}
	}

	return resps
	//	}
}

type IndexByFullTagsResp struct {
	Data []dataobj.IndexByFullTagsResp `json:"dat"`
	Err  string                        `json:"err"`
}

func (taosd *TDengineDataSource) QueryIndexByFullTags(recvs []dataobj.IndexByFullTagsRecv) ([]dataobj.IndexByFullTagsResp, int) {
	/*	if taosd.Section.config.useindex {
			var result IndexByFullTagsResp
			err := PostIndex("/api/index/counter/fullmatch", int64(taosd.Section.config.callTimeout),
				recvs, &result)
			if err != nil {
				logger.Errorf("post index failed, %+v", err)
				return nil
			}

			if result.Err != "" || len(result.Data) == 0 {
				logger.Errorf("index fullTags failed, %+v", result.Err)
				return nil
			}

			return result.Data

		} else {
	*/
	logger.Debugf("query IndexByFullTags , recv: %+v", recvs)

	c, err := NewInTaosdClient(taosd.Section)

	if err != nil {
		logger.Errorf("init taosd client fail: %v", err)
		return nil, 0
	}
	defer c.Db.Close()

	resp := make([]dataobj.IndexByFullTagsResp, 0)
	for _, recv := range recvs {
		fullTagResp := dataobj.IndexByFullTagsResp{
			Endpoints: recv.Endpoints,
			Metric:    recv.Metric,
			Tags:      make([]string, 0),
			Step:      10,
			DsType:    "GAUGE",
		}

		// 兼容夜莺逻辑，不选择endpoint则返回空
		if len(recv.Endpoints) == 0 {
			resp = append(resp, fullTagResp)
			continue
		}
		//获取超级表结构，表名以metric创建的
		stabname := MetricnameEscape(recv.Metric)
		//通过表结构获取TAGS字段名数组
		tagsMap := taosd.taosdGetTagsDescribe(c, stabname)

		// build influxql
		taosdShow := ShowSeries{
			Database:  c.Database,
			Metric:    stabname,
			Endpoints: recv.Endpoints,
			Start:     time.Now().AddDate(0, 0, -30).Unix(),
			End:       time.Now().Unix(),
		}

		taosdShow.renderEndpoints()
		taosdShow.renderTimeRange()

		taosdql := "select"
		for _, tagname := range tagsMap {
			taosdql = taosdql + fmt.Sprintf(" %s,", tagname)
		}
		taosdql = taosdql[:len(taosdql)-len(",")]

		taosdql = fmt.Sprintf("%s from  %s  %s;", taosdql, stabname, taosdShow.RawQuery)
		if taosd.Section.config.debugprt == 2 {
			fmt.Printf(taosdql)
		}
		response := taosd.taosdSQL(c, taosdql)
		if response != nil {
			for _, rowmap := range response {
				tags := make(map[string]string, 0)
				for tagkey, tagValue := range *rowmap {
					if tagValue == nil {
						continue
					}
					tags[tagkey] = tagValue.(string)
				}
				if tags != nil {
					fullTagResp.Tags = append(fullTagResp.Tags, dataobj.SortedTags(tags))
				}
			}
		} else {
			if err != nil {
				logger.Warningf("query index by full tags on taosd error %v.", err)
			}
		}
		resp = append(resp, fullTagResp)
	}

	return resp, len(resp)
	//	}
}

/*
func PostIndex(url string, calltimeout int64, recv interface{}, resp interface{}) error {
	addrs := index.IndexList.Get()
	if len(addrs) == 0 {
		logger.Errorf("empty index addr")
		return errors.New("empty index addr")
	}

	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], url)
		err = httplib.Post(url).JSONBodyQuiet(recv).SetTimeout(
			time.Duration(calltimeout) * time.Millisecond).ToJSON(&resp)
		if err == nil {
			break
		}
		logger.Warningf("index %s failed, error:%v, req:%+v", url, err, recv)
	}

	if err != nil {
		logger.Errorf("index %s failed, error:%v, req:%+v", url, err, recv)
		return err
	}
	return nil
}
*/
/*
type IndexMetricsResp struct {
	Data *dataobj.MetricResp `json:"dat"`
	Err  string              `json:"err"`
}

func (tsdb *TsdbDataSource) QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp {
	var result IndexMetricsResp
	err := PostIndex("/api/index/metrics", int64(tsdb.Section.CallTimeout), recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" {
		logger.Errorf("index xclude failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

type IndexTagPairsResp struct {
	Data []dataobj.IndexTagkvResp `json:"dat"`
	Err  string                   `json:"err"`
}

func (tsdb *TsdbDataSource) QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	var result IndexTagPairsResp
	err := PostIndex("/api/index/tagkv", int64(tsdb.Section.CallTimeout), recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" || len(result.Data) == 0 {
		logger.Errorf("index xclude failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

type IndexCludeResp struct {
	Data []dataobj.XcludeResp `json:"dat"`
	Err  string               `json:"err"`
}

func (tsdb *TsdbDataSource) QueryIndexByClude(recv []dataobj.CludeRecv) []dataobj.XcludeResp {
	var result IndexCludeResp
	err := PostIndex("/api/index/counter/clude", int64(tsdb.Section.CallTimeout), recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" || len(result.Data) == 0 {
		logger.Errorf("index xclude failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

type IndexByFullTagsResp struct {
	Data []dataobj.IndexByFullTagsResp `json:"dat"`
	Err  string                        `json:"err"`
}

func (tsdb *TsdbDataSource) QueryIndexByFullTags(recv []dataobj.IndexByFullTagsRecv) []dataobj.IndexByFullTagsResp {
	var result IndexByFullTagsResp
	err := PostIndex("/api/index/counter/fullmatch", int64(tsdb.Section.CallTimeout),
		recv, &result)
	if err != nil {
		logger.Errorf("post index failed, %+v", err)
		return nil
	}

	if result.Err != "" || len(result.Data) == 0 {
		logger.Errorf("index fullTags failed, %+v", result.Err)
		return nil
	}

	return result.Data
}

func PostIndex(url string, calltimeout int64, recv interface{}, resp interface{}) error {
	addrs := IndexList.Get()
	if len(addrs) == 0 {
		logger.Errorf("empty index addr")
		return errors.New("empty index addr")
	}

	perm := rand.Perm(len(addrs))
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s%s", addrs[perm[i]], url)
		err = httplib.Post(url).JSONBodyQuiet(recv).SetTimeout(
			time.Duration(calltimeout) * time.Millisecond).ToJSON(&resp)
		if err == nil {
			break
		}
		logger.Warningf("index %s failed, error:%v, req:%+v", url, err, recv)
	}

	if err != nil {
		logger.Errorf("index %s failed, error:%v, req:%+v", url, err, recv)
		return err
	}
	return nil
}
*/
