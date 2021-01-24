/*
 * Copyright (c) 2019 TAOS Data, Inc. <jhtao@taosdata.com>
 *
 * This program is free software: you can use, redistribute, and/or modify
 * it under the terms of the GNU Affero General Public License, version 3
 * or later ("AGPL"), as published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful, but WITHOUT
 * ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
 * FITNESS FOR A PARTICULAR PURPOSE.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */
package taosd

import (
	"container/list"
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	//"test/src/dataobj"
	//"test/src/str"
	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"
	"github.com/mitchellh/mapstructure"
	_ "github.com/taosdata/driver-go/taosSql"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
	//"github.com/toolkits/pkg/container/list"
	//"golang.org/x/sys/unix"
)

const (
	maxLocationSize = 32
	maxSQLBufSize   = 65480
)

/*
var locations = [maxLocationSize]string{
	"Beijing", "Shanghai", "Guangzhou", "Shenzhen",
	"HangZhou", "Tianjin", "Wuhan", "Changsha",
	"Nanjing", "Xian"}
*/
type nametag struct {
	tagmap   map[string]string
	taglist  *list.List
	annotlen int
}

type TaosConfiguration struct {
	daemonIP         string `yaml:"daemonIP"`         //127.0.0.1
	daemonName       string `yaml:"daemonName"`       //localhost
	serverPort       int    `yaml:"serverPort"`       //6030
	apiport          int    `yaml:"apiport"`          //6020
	dbuser           string `yaml:"dbuser"`           //root
	dbpassword       string `yaml:"dbpassword"`       //taosdata
	dbName           string `yaml:"dbName"`           //"n9e"
	taosDriverName   string `yaml:"taosDriverNmae"`   //"taosSql"
	inputworkers     int    `yaml:"inputworkers"`     //5
	insertsqlworkers int    `yaml:"insertsqlworkers"` //= 5
	//supTblName           string
	//tablePrefix          string
	//numOftables          int
	//numOfRecordsPerTable int
	//numOfRecordsPerReq   int
	//numOfThreads         int
	insertbatchSize int `yaml:"insertbatchSize"` //= 100
	inputbuffersize int `yaml:"inputbuffersize"` //= 100
	queryworkers    int `yaml:"queryworkers"`    //50
	querydatachs    int `yaml:"querydatachs"`    //1000
	insertTimeout   int `yaml:"insertTimeout"`
	callTimeout     int `yaml:"callTimeout"`
	//useindex     bool               `yaml:"useindex"`
	//Index        index.IndexSection `yaml:"index"`
	//RpcClient    rpc.RpcClientSection `yaml:"rpcClient"`
	debugprt    int `yaml:"debugprt"`    //= 2 //if 0 not print, if 1 print the sql,
	taglen      int `yaml:"taglen"`      //= 128
	taglimit    int `yaml:"taglimit"`    //= 1024
	tagnumlimit int `yaml:"tagnumlimit"` //= 128
	//	tablepervnode int
	//startTimestamp string `yaml:"startTimestamp"`
	//startTs        int64  `yaml:"startTs"`

	keep   int `yaml:"keep"`   //=365*5 //5 years数据保留期
	days   int `yaml:"days"`   //=30    //一个月1个文件
	blocks int `yaml:"blocks"` //=4     //内存块

}

type TDengineSection struct {
	Enabled bool              `yaml:"enabled"`
	Name    string            `yaml:"name"`
	config  TaosConfiguration `yaml:",inline"`
	url     string            `yaml:"url"`
	tdurl   string            `yaml:"tdurl"`
	tagstr  string            `yaml:"tagstr"`
}

type TDengineDataSource struct {
	// config
	Section TDengineSection
	//SendQueueMaxSize      int
	SendTaskSleepInterval time.Duration

	bufPool sync.Pool
	//入库缓存
	insertbatchChans []chan string //multi table one chan
	// 发送缓存队列 node ->
	inputnodeChans  []chan dataobj.MetricValue //prompb.WriteRequest //multi node one chan
	inputDone       chan struct{}
	workersGroup    sync.WaitGroup
	IsSTableCreated sync.Map
	IsTableCreated  sync.Map
}

//var taosd.Section.config config
//var taosDriverName = "taosSql"
//var url string

/*
需要实现的接口
type DataSource interface {
	PushEndpoint

	// query data for judge
	QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse
	// query data for ui
	QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse

	// query metrics & tags
	QueryMetrics(recv dataobj.EndpointsRecv) *dataobj.MetricResp
	QueryTagPairs(recv dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp
	QueryIndexByClude(recv []dataobj.CludeRecv) []dataobj.XcludeResp
	QueryIndexByFullTags(recv []dataobj.IndexByFullTagsRecv) []dataobj.IndexByFullTagsResp

	// tsdb instance
	GetInstance(metric, endpoint string, tags map[string]string) []string
}

type PushEndpoint interface {
	// push data
	Push2Queue(items []*dataobj.MetricValue)
}
数据格式
//设备相关采用 endpoint做唯一标识
tags 字串模式
[
    {
        "metric": "disk.io.util",
        "endpoint": "10.86.12.13",
        "tags": "device=sda",
        "value": 15.4,
        "timestamp": 1554455574,
        "step": 20
    },
    {
        "metric": "api.latency",
        "endpoint": "10.86.12.13",
        "tags": "api=/api/v1/auth/login,srv=n9e,mod=monapi,idc=bj",
        "value": 5.4,
        "timestamp": 1554455574,
        "step": 20
    }
]

tagsMap格式
[
    {
        "metric": "disk.io.util",
        "endpoint": "10.86.12.13",
        "tagsMap": {
        	"device": "sda"
        },
        "value": 15.4,
        "timestamp": 1554455574,
        "step": 20
    },
    {
        "metric": "api.latency",
        "endpoint": "10.86.12.13",
        "tagsMap": {
        	"api": "/api/v1/auth/login",
        	"srv": "n9e",
        	"mod": "monapi",
        	"idc": "bj"
        },
        "value": 5.4,
        "timestamp": 1554455574,
        "step": 20
    }
]
//设备无关采用 nid做唯一标识
[
    {
        "metric": "aggr.api.latency.ms",
        "nid": "162",
        "tagsMap": {
        	"api": "/api/v1/auth/login",
        	"srv": "n9e",
        	"mod": "monapi",
        	"idc": "bj"
        },
        "value": 5.4,
        "timestamp": 1554455574,
        "step": 20
    }
]

//具体数及结构
见common/dataobject/metric.go
type MetricValue struct {
	Nid          string            `json:"nid"`
	Metric       string            `json:"metric"`
	Endpoint     string            `json:"endpoint"`
	Timestamp    int64             `json:"timestamp"`
	Step         int64             `json:"step"`
	ValueUntyped interface{}       `json:"value"`
	Value        float64           `json:"-"`
	CounterType  string            `json:"counterType"`
	Tags         string            `json:"tags"`
	TagsMap      map[string]string `json:"tagsMap"` //保留2种格式，方便后端组件使用
	Extra        string            `json:"extra"`
}
*/
func (taosd *TDengineDataSource) Init() {

	logger.Debugf("taosd datasource init")
	//startTs, err := time.ParseInLocation("2006-01-02 15:04:05", taosd.Section.config.startTimestamp, time.Local)
	//taosd.Section.config.startTs = startTs
	//if err == nil {
	//	taosd.Section.config.startTs = taosd.Section.config.startTs.UnixNano() / 1e6
	//}
	if taosd.Section.config.daemonName != "" {
		s, _ := net.LookupIP(taosd.Section.config.daemonName)
		if s != nil {
			taosd.Section.config.daemonIP = fmt.Sprintf("%s", s[0])

			//fmt.Println(daemonIP)
			//fmt.Println(s[0])
			//taosd.Section.config.daemonIP = daemonIP + ":0"

			taosd.Section.tdurl = taosd.Section.config.daemonName
		} else {
			logger.Errorf("daemonName %s is should not to use", taosd.Section.config.daemonName)
			taosd.Section.tdurl = taosd.Section.config.daemonIP
		}
	} else {
		taosd.Section.tdurl = taosd.Section.config.daemonIP
		//daemonIP = taosd.Section.config.daemonIP + ":0"
	}

	taosd.Section.tagstr = fmt.Sprintf(" binary(%d)", taosd.Section.config.taglen)

	//taosd.Section.url = fmt.Sprintf("%s:%s@/tcp(%s:%d)/%s?interpolateParams=true", taosd.Section.config.dbuser, taosd.Section.config.dbpassword, taosd.Section.config.daemonIP, taosd.Section.config.serverPort, taosd.Section.config.dbName)
	taosd.Section.url = fmt.Sprintf("%s:%s@/tcp(%s:%d)/", taosd.Section.config.dbuser, taosd.Section.config.dbpassword, taosd.Section.config.daemonIP, taosd.Section.config.serverPort)

	logger.Debugf("URL:%s", taosd.Section.url)
	if taosd.Section.config.debugprt == 2 {
		taosd.printAllArgs()
	}
	//添加Metrics写请求通道
	for i := 0; i < taosd.Section.config.inputworkers; i++ {
		taosd.inputnodeChans = append(taosd.inputnodeChans, make(chan dataobj.MetricValue, taosd.Section.config.inputbuffersize))
	}
	//如果数据库不存在就建立数据库，此处为n9e
	taosd.createDatabase(taosd.Section.config.dbName)
	//
	for i := 0; i < taosd.Section.config.inputworkers; i++ {
		taosd.workersGroup.Add(1)
		go taosd.NodeProcess(i)
	}
	// 创建入库SQL语句通道 并并发workergroup
	if taosd.Section.Enabled {
		for i := 0; i < taosd.Section.config.insertsqlworkers; i++ {
			taosd.insertbatchChans = append(taosd.insertbatchChans, make(chan string, taosd.Section.config.insertbatchSize))
		}
		// init task
		for i := 0; i < taosd.Section.config.insertsqlworkers; i++ {
			taosd.workersGroup.Add(1)
			go taosd.processBatches(i)
		}
	}
	/*
		if taosd.Section.config.useindex {
			index.Init(taosd.Section.config.Index)
			//time.Sleep(time.Second * 10)
			brpc.Init(taosd.Section.config.RpcClient, index.IndexList.Get())
			//go index.GetIndexLoop()
		}
	*/
}

//创建数据库，
//CREATE DATABASE power KEEP 365 DAYS 10 BLOCKS 4;
//上述语句将创建一个名为power的库，这个库的数据将保留365天（超过365天将被自动删除），
//每10天一个数据文件，内存块数为4。详细的语法及参数请见TAOS SQL
/*
修改数据库参数

ALTER DATABASE db_name COMP 2;
COMP参数是指修改数据库文件压缩标志位，取值范围为[0, 2]. 0表示不压缩，1表示一阶段压缩，2表示两阶段压缩。

ALTER DATABASE db_name REPLICA 2;
REPLICA参数是指修改数据库副本数，取值范围[1, 3]。在集群中使用，副本数必须小于或等于dnode的数目。

ALTER DATABASE db_name KEEP 365;
KEEP参数是指修改数据文件保存的天数，缺省值为3650，取值范围[days, 365000]，必须大于或等于days参数值。

ALTER DATABASE db_name QUORUM 2;
QUORUM参数是指数据写入成功所需要的确认数。取值范围[1, 3]。对于异步复制，quorum设为1，具有master角色的虚拟节点自己确认即可。对于同步复制，需要至少大于等于2。原则上，Quorum >=1 并且 Quorum <= replica(副本数)，这个参数在启动一个同步模块实例时需要提供。

ALTER DATABASE db_name BLOCKS 100;
BLOCKS参数是每个VNODE (TSDB) 中有多少cache大小的内存块，因此一个VNODE的用的内存大小粗略为（cache * blocks）。取值范围[3, 1000]。
*/
func (taosd *TDengineDataSource) createDatabase(dbname string) {
	//db, err := sql.Open(taosDriverName, dbuser+":"+dbpassword+"@/tcp("+daemonIP+")/")
	db, err := sql.Open(taosd.Section.config.taosDriverName, taosd.Section.url)

	if err != nil {
		logger.Fatalf("Open database error: %s\n", err)
	}
	defer db.Close()
	//sqlStr = "create database " + dbName + " keep " + strconv.Itoa(taosd.Section.config.keep) + " days " + strconv.Itoa(taosd.Section.config.days)

	sqlcmd := fmt.Sprintf("create database if not exists %s ", dbname)
	sqlcmd = sqlcmd + " keep " + strconv.Itoa(taosd.Section.config.keep) + " days " + strconv.Itoa(taosd.Section.config.days) + " BLOCKS " + strconv.Itoa(taosd.Section.config.blocks) + ";"
	_, err = db.Exec(sqlcmd)
	if err != nil {
		logger.Fatalf("create database error: %s\n", err)
	}
	sqlcmd = fmt.Sprintf("use %s;", dbname)
	_, err = db.Exec(sqlcmd)
	taosd.checkErr(err)
	return
}

func (taosd *TDengineDataSource) execSql(dbname string, sqlcmd string, db *sql.DB) (sql.Result, error) {
	if len(sqlcmd) < 1 {
		return nil, nil
	}
	if taosd.Section.config.debugprt == 2 {
		logger.Debug(sqlcmd)
	}
	res, err := db.Exec(sqlcmd)
	if err != nil {
		logger.Errorf("execSql Error: %s sqlcmd: %s\n", err, sqlcmd)

	}
	return res, err
}

func (taosd *TDengineDataSource) checkErr(err error) {
	if err != nil {
		logger.Error(err)
	}
}
func (taosd *TDengineDataSource) TAOShashID(ba []byte) int {
	var sum int = 0
	for i := 0; i < len(ba); i++ {
		sum += int(ba[i] - '0')
	}
	return sum
}
func (taosd *TDengineDataSource) md5V2(str string) string {
	data := []byte(str)
	has := md5.Sum(data)
	md5str := fmt.Sprintf("%x", has)
	return md5str
}

// 打到 Tsdb 的数据,要根据 rrdtool 的特定 来限制 step、counterType、timestamp
func (taosd *TDengineDataSource) convert2TsdbItem(d *dataobj.MetricValue) *dataobj.TsdbItem {
	item := &dataobj.TsdbItem{
		Nid:       d.Nid,
		Endpoint:  d.Endpoint,
		Metric:    d.Metric,
		Value:     d.Value,
		Timestamp: d.Timestamp,
		Tags:      d.Tags,
		TagsMap:   d.TagsMap,
		Step:      int(d.Step),
		Heartbeat: int(d.Step) * 2,
		DsType:    dataobj.GAUGE,
		Min:       "U",
		Max:       "U",
	}

	return item
}

type Point struct {
	Key       string  `msg:"key"`
	Timestamp int64   `msg:"timestamp"`
	Value     float64 `msg:"value"`
}

func (taosd *TDengineDataSource) convert2CacheServerItem(d *dataobj.TsdbItem) Point {
	if d.Nid != "" {
		d.Endpoint = dataobj.NidToEndpoint(d.Nid)
	}
	p := Point{
		Key:       str.Checksum(d.Endpoint, d.Metric, str.SortedTags(d.TagsMap)),
		Timestamp: d.Timestamp,
		Value:     d.Value,
	}
	return p
}

// 将原始数据插入到taosd请求缓存队列，此处改写自http接入，以后可拓展支持各种数据来源
//此接口由transfer调用
func (taosd *TDengineDataSource) Push2Queue(items []*dataobj.MetricValue) {
	errCnt := 0
	var req dataobj.MetricValue
	for _, item := range items {
		req = *item
		var address string

		//取Endpoint或Nid作为唯一标识经过和计算后为通道索引键值
		if item.Endpoint != "" {
			address = item.Endpoint
		} else {
			if item.Nid != "" {
				address = item.Nid
			} else {
				continue
			}
		}
		if address != "" {
			stats.Counter.Set("points.in", 1)
			logger.Debugf("taosd Push2Queue :%s", address)
			if taosd.Section.config.debugprt == 2 {
				fmt.Println("taosd Push2Queue:", req)
			}
			//index.ReceiveItem(item, item.Key)
			/*
				if taosd.Section.config.useindex {
					tsdbItem := taosd.convert2TsdbItem(item)
					stats.Counter.Set("taosd.queue.push", 1)
					point := taosd.convert2CacheServerItem(tsdbItem)
					index.ReceiveItem(tsdbItem, point.Key)
					logger.Debugf("taosd Push2Index :%s", address)
					fmt.Println("taosd Push2Index:", req)
				}
			*/
			idx := taosd.TAOShashID([]byte(address))
			taosd.inputnodeChans[idx%taosd.Section.config.inputworkers] <- req //将需入库参数组写入通道队列
		} else {
			errCnt += 1
		}
	}
	stats.Counter.Set("taosdb.inputnodeChans.err", errCnt)
}

func (taosd *TDengineDataSource) NodeProcess(workerid int) error {
	db, err := sql.Open(taosd.Section.config.taosDriverName, taosd.Section.url+taosd.Section.config.dbName) // taosd.Section.config.dbuser+":"+taosd.Section.config.dbpassword+"@/tcp("+taosd.Section.config.hostName+")/"+taosd.Section.config.dbName)
	if err != nil {
		logger.Errorf("ProcessReq Open database error: %s\n", err)
		//logger.Errorf("processBatches Open database error: %s\n", err)
		var count int = 5
		for {
			if err != nil && count > 0 {
				<-time.After(time.Second * 1)
				_, err = sql.Open(taosd.Section.config.taosDriverName, taosd.Section.url+taosd.Section.config.dbName) // taosd.Section.config.dbuser+":"+taosd.Section.config.dbpassword+"@/tcp("+taosd.Section.config.hostName+")/"+taosd.Section.config.dbName)
				count--
			} else {
				if err != nil {
					logger.Errorf("ProcessReq Error: %s open database\n", err)
					//logger.Errorf("processBatches Error: %s open database\n", err)
					return err
				}
				break
			}
		}
	}

	defer db.Close()

	for req := range taosd.inputnodeChans[workerid] {

		taosd.ProcessReq(req, db)

	}

	return nil
}

//处理入库请求
func (taosd *TDengineDataSource) ProcessReq(req dataobj.MetricValue, db *sql.DB) error {

	//for _, ts := range req {
	err := taosd.HandleStable(req, db)

	return err

}

//创建超级表或者根据tagsMap添加超级表的tags
//表结构
/*
type MetricValue struct {
	Nid          string            `json:"nid"`
	Metric       string            `json:"metric"`
	Endpoint     string            `json:"endpoint"`
	Timestamp    int64             `json:"timestamp"`
	Step         int64             `json:"step"`
	ValueUntyped interface{}       `json:"value"`
	Value        float64           `json:"-"`
	CounterType  string            `json:"counterType"`
	Tags         string            `json:"tags"`
	TagsMap      map[string]string `json:"tagsMap"` //保留2种格式，方便后端组件使用
	Extra        string            `json:"extra"`
}
数据格式
//设备相关采用 endpoint做唯一标识
tags 字串模式
[
    {
        "metric": "disk.io.util",
        "endpoint": "10.86.12.13",
        "tags": "device=sda",
        "value": 15.4,
        "timestamp": 1554455574,
        "step": 20
    },
    {
        "metric": "api.latency",
        "endpoint": "10.86.12.13",
        "tags": "api=/api/v1/auth/login,srv=n9e,mod=monapi,idc=bj",
        "value": 5.4,
        "timestamp": 1554455574,
        "step": 20
    }
]

tagsMap格式
[
    {
        "metric": "disk.io.util",
        "endpoint": "10.86.12.13",
        "tagsMap": {
        	"device": "sda"
        },
        "value": 15.4,
        "timestamp": 1554455574,
        "step": 20
    },
    {
        "metric": "api.latency",
        "endpoint": "10.86.12.13",
        "tagsMap": {
        	"api": "/api/v1/auth/login",
        	"srv": "n9e",
        	"mod": "monapi",
        	"idc": "bj"
        },
        "value": 5.4,
        "timestamp": 1554455574,
        "step": 20
    }
]
//设备无关采用 nid做唯一标识
[
    {
        "metric": "aggr.api.latency.ms",
        "nid": "162",
        "tagsMap": {
        	"api": "/api/v1/auth/login",
        	"srv": "n9e",
        	"mod": "monapi",
        	"idc": "bj"
        },
        "value": 5.4,
        "timestamp": 1554455574,
        "step": 20
    }
]
{
	"fields":{
	"usage_guest":0,
	"usage_guest_nice":0,
	"usage_idle":87.73726273726274,
	"usage_iowait":0,
	"usage_irq":0,
	"usage_nice":0,
	"usage_softirq":0,
	"usage_steal":0,
	"usage_system":2.6973026973026974,
	"usage_user":9.565434565434565
	},
	"name":"cpu",
	"tags":{
		"cpu":"cpu-total",
		"host":"liutaodeMacBook-Pro.local"
		},
	"timestamp":1571665100
}
func (influxdb *InfluxdbDataSource) convert2InfluxdbItem(d *dataobj.MetricValue) *dataobj.InfluxdbItem {
	t := dataobj.InfluxdbItem{Tags: make(map[string]string), Fields: make(map[string]interface{})}

	for k, v := range d.TagsMap {
		t.Tags[k] = v
	}
	t.Tags["endpoint"] = d.Endpoint
	t.Measurement = d.Metric
	t.Fields["value"] = d.Value
	t.Timestamp = d.Timestamp

	return &t
}

*/
func (taosd *TDengineDataSource) autochangevalue(valueuntype interface{}) (outvalue dataobj.JsonFloat, err error) {
	switch valueuntype.(type) {
	case int32:
		outvalue = dataobj.JsonFloat(valueuntype.(int32))
	case string:
		//outvalue=valueuntype.(string)
	case float32:
		outvalue = dataobj.JsonFloat(valueuntype.(float32))
	case float64:
		outvalue = dataobj.JsonFloat(valueuntype.(float64))
	case uint64:
		outvalue = dataobj.JsonFloat(valueuntype.(uint64))
	case int64:
		outvalue = dataobj.JsonFloat(valueuntype.(int64))
	case int:
		outvalue = dataobj.JsonFloat(valueuntype.(int))
	case bool:
		outvalue = dataobj.JsonFloat(valueuntype.(int8))
	case int8:
		outvalue = dataobj.JsonFloat(valueuntype.(int8))
	case int16:
		outvalue = dataobj.JsonFloat(valueuntype.(int16))
	case byte:

	default:
		//	var err error
		//err.(string) = "not support"
		//fmt.Printf()err("not support")
		return 0, errors.New("not support")
	}
	return outvalue, nil
}
func (taosd *TDengineDataSource) autogencreatetabelsql(stbname string, metric dataobj.MetricValue) (sqlcmd string, err error) {
	var sqlbody string
	switch metric.ValueUntyped.(type) {
	case string:
		sqlbody = " (ts timestamp, value BINARY(100)) tags(taghash binary(34)"
	case float32:
		sqlbody = " (ts timestamp, value FLOAT) tags(taghash binary(34)"
	case float64:
		sqlbody = " (ts timestamp, value DOUBLE) tags(taghash binary(34)"
	case uint64:
		sqlbody = " (ts timestamp, value BIGINT) tags(taghash binary(34)"
	case int64:
		sqlbody = " (ts timestamp, value BIGINT) tags(taghash binary(34)"
	case int:
		sqlbody = " (ts timestamp, value INT) tags(taghash binary(34)"
	case bool:
		sqlbody = " (ts timestamp, value BOOL) tags(taghash binary(34)"
	case int8:
		sqlbody = " (ts timestamp, value TINYINT) tags(taghash binary(34)"
	case int16:
		sqlbody = " (ts timestamp, value SMALLINT) tags(taghash binary(34)"
	case byte:

	default:
		//	var err error
		//err.(string) = "not support"
		//fmt.Printf()err("not support")
		return "", errors.New("not support")
	}
	//var
	sqlcmd = "create table if not exists " + stbname + sqlbody
	return sqlcmd, nil
}
func (taosd *TDengineDataSource) HandleStable(metric dataobj.MetricValue, db *sql.DB) error {
	taglist := list.New()   // save push in data tags name include endpoint / nid
	tbtaglist := list.New() //save ready add to stable table  tags max num(tagnumlimit)
	tagmap := make(map[string]string)
	tbtagmap := make(map[string]string)
	//m := make(metric, len(metric))
	tagnum := taosd.Section.config.tagnumlimit
	//var hasName bool = false
	var metricsName string
	var tbn string = ""
	var ln string = ""
	taglen := taosd.Section.config.taglen
	var nt nametag
	var sqlcmd string
	//var annotlen int
	//fmt.Println(ts)
	j := 0
	metricsName = metric.Metric
	if metricsName != "" {
		tbn += metricsName
		//hasName = true
	} else {
		info := fmt.Sprintf("no name metric")
		logger.Errorf(info)
		return nil
	}
	if metric.Endpoint != "" {
		tagmap["endpoint"] = metric.Endpoint
		ln = "endpoint" //strings.ToLower(string(metric.Endpoint))
		taosd.OrderInsertS(ln, taglist)
		taosd.OrderInsertS(ln, tbtaglist)
		tbtagmap[ln] = "y"
		tbn += strings.ToLower(string(metric.Endpoint))
		j++
	} else {
		if metric.Nid != "" {
			tagmap["nid"] = metric.Nid
			ln = "nid" //strings.ToLower(string(metric.Nid))
			taosd.OrderInsertS(ln, taglist)
			taosd.OrderInsertS(ln, tbtaglist)
			tbtagmap[ln] = "y"
			tbn += strings.ToLower(string(metric.Nid))
			j++
		}
	}
	for k, v := range metric.TagsMap {
		//j <= tagnum
		j++
		ln = strings.ToLower(string(k))
		//taosd.OrderInsertS(ln, taglist)
		s := string(v)
		if j <= tagnum {
			tbn += s
			taosd.OrderInsertS(ln, taglist)
			taosd.OrderInsertS(ln, tbtaglist)
			//tbtaglist.PushBack(ln)
			if len(s) > taglen {
				s = s[:taglen]
			}
			tagmap[ln] = s
			tbtagmap[ln] = "y"
		}

	}

	if taosd.Section.config.debugprt == 2 {
		t := metric.Timestamp
		//var ns int64 = 0
		//if t/1000000000 > 10 {
		//	tm := t / 1000
		//	ns = t - tm*1000
		//}
		logger.Debugf(" Ts: %v, value: %v, ", time.Unix(t, 0), metric.Value)
		//logger.Debug(ts)
	}

	stbname := taosd.tablenameEscape(metricsName)
	//var ok bool
	//i := 0
	schema, ok := taosd.IsSTableCreated.Load(stbname)
	if !ok { // no local record of super table structure
		//获取表结构的TAG字段数组
		stablehas := false
		tags := taosd.taosdGetTableTagDescribe(db, stbname)
		if tags != nil {
			taostaglist := list.New()
			taostagmap := make(map[string]string)
			for _, tag := range tags {
				taostaglist.PushBack(tag)
				taostagmap[tag] = "y"
			}
			nt.taglist = taostaglist
			nt.tagmap = taostagmap
			//tbtaglist = nt.taglist
			//tbtagmap = nt.tagmap
			stablehas = true //stable is exist

		}
		//nt.tagmap
		//nt.taglist

		if stablehas { //超级表存在
			//需要插入的 tags
			//yes, the super table was already created in TDengine
			for e := tbtaglist.Front(); e != nil; e = e.Next() {
				k := e.Value.(string)
				//i++
				//if i < taosd.Section.config.tagnumlimit {
				_, ok = nt.tagmap[k] //表结构的tags name
				if !ok {
					//tag以前不存在，需要添补插入，tag在超级表里没找到则改变表结构，添加此tag
					sqlcmd = "alter table " + stbname + " add tag " + k + taosd.Section.tagstr + "\n"
					_, err := taosd.execSql(taosd.Section.config.dbName, sqlcmd, db)
					if err != nil {
						logger.Error(err)
						errorcode := fmt.Sprintf("%s", err)
						if strings.Contains(errorcode, "duplicated column names") {
							nt.taglist.PushBack(k)
							//OrderInsertS(k, tbtaglist)
							nt.tagmap[k] = "y"
						}
					} else {
						nt.taglist.PushBack(k)
						//OrderInsertS(k, tbtaglist)
						nt.tagmap[k] = "y"
					}
				}
				//}
			}
			tbtaglist = nt.taglist
			tbtagmap = nt.tagmap
			taosd.IsSTableCreated.Store(stbname, nt)
		} else { //超级表不存在
			// no, the super table haven't been created in TDengine, create it.
			nt.taglist = tbtaglist
			nt.tagmap = tbtagmap
			sqlcmd, err := taosd.autogencreatetabelsql(stbname, metric)
			if err != nil {
				logger.Errorf("autogencreatetabelsql%s", err.Error())
				return err
			}
			//sqlcmd = "create table if not exists " + stbname + " (ts timestamp, value double) tags(taghash binary(34)"
			for e := tbtaglist.Front(); e != nil; e = e.Next() {
				sqlcmd = sqlcmd + "," + e.Value.(string) + taosd.Section.tagstr
			}
			sqlcmd = sqlcmd + ")\n"
			if taosd.Section.config.debugprt == 2 {
				fmt.Printf("SQL:%s", sqlcmd)
			}
			_, err = taosd.execSql(taosd.Section.config.dbName, sqlcmd, db)
			if err == nil {
				//tbtaglist = nt.taglist
				//tbtagmap = nt.tagmap
				taosd.IsSTableCreated.Store(stbname, nt)
			} else {
				logger.Error(err)
				return err
			}
		}
	} else { //有本地 tag信息，就让需要插入的tags跟本地的tag信息进行比对
		ntag := schema.(nametag)
		//tbtaglist = ntag.taglist
		//tbtagmap = ntag.tagmap
		//i := 0
		for e := tbtaglist.Front(); e != nil; e = e.Next() {
			k := e.Value.(string)
			//i++
			//if i < taosd.Section.config.tagnumlimit {
			_, ok := ntag.tagmap[k]
			if !ok {
				sqlcmd = "alter table " + stbname + " add tag " + k + taosd.Section.tagstr + "\n"
				_, err := taosd.execSql(taosd.Section.config.dbName, sqlcmd, db)
				if err != nil {
					logger.Error(err)
					errorcode := fmt.Sprintf("%s", err)
					if strings.Contains(errorcode, "duplicated column names") {
						ntag.taglist.PushBack(k)
						//OrderInsertS(k, tbtaglist)
						ntag.tagmap[k] = "y"
					} else {
						return err
					}

				} else {
					ntag.taglist.PushBack(k)
					//OrderInsertS(k, tbtaglist)
					ntag.tagmap[k] = "y"
				}
			}
			//}
		}

		tbtaglist = ntag.taglist
		tbtagmap = ntag.tagmap
		taosd.IsSTableCreated.Store(stbname, ntag)
	}
	// insert device table data ,tables create auto
	tbnhash := "MD5_" + taosd.md5V2(tbn)
	_, tbcreated := taosd.IsTableCreated.Load(tbnhash)
	//tbtaglist,tbtagmap根据更新过程记录最新的tag结构
	if !tbcreated {
		var sqlcmdhead, sqlcmd string
		sqlcmdhead = "create table if not exists " + tbnhash + " using " + stbname + " tags(\""
		sqlcmd = ""
		i := 0
		for e := tbtaglist.Front(); e != nil; e = e.Next() {
			if e.Value.(string) == "taghash" {
				continue
			}
			tagvalue, has := tagmap[e.Value.(string)]
			if len(tagvalue) > taglen {
				tagvalue = tagvalue[:taglen]
			}

			if i == 0 {
				if has {
					sqlcmd = sqlcmd + "\"" + tagvalue + "\""
				} else {
					sqlcmd = sqlcmd + "null"
				}
				i++
			} else {
				if has {
					sqlcmd = sqlcmd + ",\"" + tagvalue + "\""
				} else {
					sqlcmd = sqlcmd + ",null"
				}
			}

		}

		var keys []string
		var tagHash = ""
		for t := range tagmap {
			keys = append(keys, t)
		}
		sort.Strings(keys)
		for _, k := range keys {
			tagHash += tagmap[k]
		}

		sqlcmd = sqlcmd + ")\n"
		sqlcmd = sqlcmdhead + taosd.md5V2(tagHash) + "\"," + sqlcmd
		_, err := taosd.execSql(taosd.Section.config.dbName, sqlcmd, db)
		if err == nil {
			taosd.IsTableCreated.Store(tbnhash, true)
		} else {
			return err
		}
	}
	if taosd.Section.config.debugprt == 2 {
		fmt.Println("stable:", metric)
	}
	taosd.serilizeTDengine(metric, tbnhash, db)
	return nil
}

func (taosd *TDengineDataSource) queryTableStruct(tbname string) string {
	client := new(http.Client)
	s := fmt.Sprintf("describe %s.%s", taosd.Section.config.dbName, tbname)
	body := strings.NewReader(s)
	url := "http://" + taosd.Section.tdurl + ":" + fmt.Sprintf("%d", taosd.Section.config.apiport) + "/rest/sql"
	req, _ := http.NewRequest("GET", url, body)
	//fmt.Println("http://" + tdurl + ":" + apiport + "/rest/sql" + s)
	req.SetBasicAuth(taosd.Section.config.dbuser, taosd.Section.config.dbpassword)
	resp, err := client.Do(req)

	if err != nil {
		logger.Error(err)
		return ""
	} else {
		compressed, _ := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		return string(compressed)
	}
}

func (taosd *TDengineDataSource) OrderInsertS(s string, l *list.List) {
	e := l.Front()
	if e == nil {
		l.PushFront(s)
		return
	}

	for e = l.Front(); e != nil; e = e.Next() {
		str := e.Value.(string)

		if taosd.TAOSstrCmp(str, s) {
			continue
		} else {
			l.InsertBefore(s, e)
			return
		}
	}
	l.PushBack(s)
	return
}
func (taosd *TDengineDataSource) tablenameEscape(mn string) string {
	stbb := strings.ReplaceAll(mn, ":", "_") // replace : in the metrics name to adapt the TDengine
	stbc := strings.ReplaceAll(stbb, ".", "_")
	stbd := strings.ReplaceAll(stbc, "-", "_")
	stbname := strings.ReplaceAll(stbd, "/", "_")
	if len(stbname) > 190 {
		stbname = stbname[:190]
	}
	return stbname
}

func MetricnameEscape(mn string) string {
	stbname := strings.ReplaceAll(mn, ".", "_") // replace : in the metrics name to adapt the TDengine
	//stbc := strings.ReplaceAll(stbb, ".", "_")
	//stbname := strings.ReplaceAll(stbc, "-", "_")
	//stbname := strings.ReplaceAll(stbc, "/", "_")
	if len(stbname) > 190 {
		stbname = stbname[:190]
	}
	return stbname
}

func UnmetricnameEscape(mn string) string {
	metric := strings.ReplaceAll(mn, "_", ".") // replace : in the metrics name to adapt the TDengine
	//stbc := strings.ReplaceAll(stbb, ".", "_")
	//stbname := strings.ReplaceAll(stbc, "-", "_")
	//stbname := strings.ReplaceAll(stbc, "/", "_")
	if len(metric) > 190 {
		metric = metric[:190]
	}
	return metric
}

type PairData struct {
	Timestamp int64             `json:"timestamp"`
	Value     dataobj.JsonFloat `json:"value"`
}
type IotPairData struct {
	endpoint string
	Tags     string
	pairdata PairData
}

type listItem struct {
	id   int
	name string
}

/*
func (taosd *TDengineDataSource) divgroups(queryrespstr []*map[string]interface{}) map[string]*list.List {
	if queryrespstr == nil {
		return nil
	}
	logger.Debugf("database query result: ")
	logger.Debug(queryrespstr)
	m := make(map[string]*list.List, 10)
	response := queryrespstr
	for _, resultmap := range response {

		var iotpairdata IotPairData
		//var l list.List
		//l := list.New()
		for k, v := range *resultmap {
			if k == "endpoint" {
				iotpairdata.endpoint = v.(string)
				continue
			} else if string.Contains(k, "t_") {
				iotpairdata.Tags = iotpairdata.Tags + k + "=" + v + ","
				continue
			} else if k == "value" {
				iotpairdata.PairData.Value = v
				continue
			} else if k == "ts" {
				iotpairdata.PairData.Timestamp = v
			}
		}
		if list != nillist {
			list.push(iotpairdata)
			m[iotpairdata.endpoint] = list
			list = nil
		}
	}
	return m
}
*/
func (taosd *TDengineDataSource) serilizeTDengine(m dataobj.MetricValue, tbn string, db *sql.DB) error {
	idx := taosd.TAOShashID([]byte(tbn))
	sqlcmd := " " + tbn + " values("
	vl := m.Value
	vls := strconv.FormatFloat(vl, 'E', -1, 64)

	if vls == "NaN" {
		vls = "null"
	}
	tl := m.Timestamp
	tls := strconv.FormatInt(tl, 10)
	sqlcmd = sqlcmd + tls + "000" + "," + vls + ")\n"
	id := idx % taosd.Section.config.insertsqlworkers
	logger.Debugf("Send-->GO %v"+sqlcmd, id)
	taosd.insertbatchChans[id] <- sqlcmd
	return nil
}

//批量任务执行器
func (taosd *TDengineDataSource) processBatches(iworker int) {
	var i int
	db, err := sql.Open(taosd.Section.config.taosDriverName, taosd.Section.url+taosd.Section.config.dbName) //taosd.Section.config.dbuser+":"+taosd.Section.config.dbpassword+"@/tcp("+taosd.Section.config.hostName+")/"+taosd.Section.config.dbName)
	if err != nil {
		logger.Errorf("processBatches Open database error: %s\n", err)
		//logger.Errorf("processBatches Open database error: %s\n", err)
		var count int = 5
		for {
			if err != nil && count > 0 {
				<-time.After(time.Second * 1)
				_, err = sql.Open(taosd.Section.config.taosDriverName, taosd.Section.url+taosd.Section.config.dbName) // taosd.Section.config.dbuser+":"+taosd.Section.config.dbpassword+"@/tcp("+taosd.Section.config.hostName+")/"+taosd.Section.config.dbName)
				count--
			} else {
				if err != nil {
					logger.Errorf("processBatches Error: %s open database\n", err)
					//logger.Errorf("processBatches Error: %s open database\n", err)
					return
				}
				break
			}
		}
	}
	defer db.Close()
	insertbatchSize := taosd.Section.config.insertbatchSize
	if insertbatchSize <= 0 {
		insertbatchSize = 10
	}
	timerout := taosd.Section.config.insertTimeout
	if timerout <= 0 {
		timerout = 5
	}
	second := time.Second
	timer := second * time.Duration(timerout)
	sqlcmd := make([]string, insertbatchSize+1)
	i = 0
	//sqlcmdHead := "Insert into"
	sqlcmd[i] = "Insert into"
	i++
	/*
		//首先，实现并执行一个匿名的超时等待函数
		timeout := make(chan bool, 1)
		go func() {
			time.Sleep(1e9)	//等待1秒钟
			timeout <- true
		}()
		//然后，我们把timeout这个channel利用起来
		select {
				case <- ch:
					//从ch中读到数据
				case <- timeout:
					//一直没有从ch中读取到数据，但从timeout中读取到数据
			}
	*/
	//logger.Errorf("processBatches")
	//timeout := time.After(time.Second * 10)
	for {
		select {
		case onepoint := <-taosd.insertbatchChans[iworker]:
			{
				sqlcmd[i] = onepoint
				i++
				if i > insertbatchSize {
					i = 1
					logger.Debugf(strings.Join(sqlcmd, "")+"buffer full batch SQL %v", iworker)
					if taosd.Section.config.debugprt == 2 {
						fmt.Println("insertbatchSize full:", strings.Join(sqlcmd, ""))
					}
					_, err := db.Exec(strings.Join(sqlcmd, ""))
					if err != nil {
						logger.Errorf("processBatches error %v", err)
						var count int = 2
						for {
							if err != nil && count > 0 {
								<-time.After(time.Second * 1)
								_, err = db.Exec(strings.Join(sqlcmd, ""))
								count--
							} else {
								if err != nil {
									logger.Errorf("Error: %v sqlcmd: %v\n", err, strings.Join(sqlcmd, ""))
								}
								break
							}

						}
					}
				}
			}
			//time.After(time.Second * 1)
		case <-time.After(timer):
			if i > 1 {
				i = 1
				logger.Debugf(strings.Join(sqlcmd, "")+"timer out batch SQL %v", iworker)
				if taosd.Section.config.debugprt == 2 {
					fmt.Println("timerout:", strings.Join(sqlcmd, ""))
				}
				_, err := db.Exec(strings.Join(sqlcmd, ""))
				if err != nil {
					var count int = 2
					for {
						if err != nil && count > 0 {
							<-time.After(time.Second * 1)
							_, err = db.Exec(strings.Join(sqlcmd, ""))
							count--
						} else {
							if err != nil {
								logger.Errorf("Error: %v sqlcmd: %v\n", err, strings.Join(sqlcmd, ""))
							}
							break
						}
					}
				}
			}

		}
	}
	/*
		for onepoint := range taosd.insertbatchChans[iworker] {
			sqlcmd[i] = onepoint
			i++
			if i > insertbatchSize {
				i = 1
				logger.Debugf(strings.Join(sqlcmd, "")+"batch SQL %v", iworker)
				//fmt.Printf("insertbatchSize max processBatches:", strings.Join(sqlcmd, ""))
				_, err := db.Exec(strings.Join(sqlcmd, ""))
				if err != nil {
					logger.Errorf("processBatches error %v", err)
					var count int = 2
					for {
						if err != nil && count > 0 {
							<-time.After(time.Second * 1)
							_, err = db.Exec(strings.Join(sqlcmd, ""))
							count--
						} else {
							if err != nil {
								logger.Errorf("Error: %v sqlcmd: %v\n", err, strings.Join(sqlcmd, ""))
							}
							break
						}

					}
				}
			}
		}
		if i > 1 {
			i = 1
			//		logger.Errorf(strings.Join(sqlcmd, ""))
			//fmt.Printf("processBatches:%v", strings.Join(sqlcmd, ""))
			logger.Debugf(strings.Join(sqlcmd, "")+"batch SQL %v", iworker)
			_, err := db.Exec(strings.Join(sqlcmd, ""))
			if err != nil {
				var count int = 2
				for {
					if err != nil && count > 0 {
						<-time.After(time.Second * 1)
						_, err = db.Exec(strings.Join(sqlcmd, ""))
						count--
					} else {
						if err != nil {
							logger.Errorf("Error: %v sqlcmd: %v\n", err, strings.Join(sqlcmd, ""))
						}
						break
					}
				}
			}
		}
	*/

	taosd.workersGroup.Done()
}

func (taosd *TDengineDataSource) printAllArgs() {
	logger.Debug("\n============= args parse result: =============\n")
	logger.Debug("daemonName:\n", taosd.Section.config.daemonName)
	logger.Debug("serverPort:\n", taosd.Section.config.serverPort)
	logger.Debug("usr:\n", taosd.Section.config.dbuser)
	logger.Debug("password:\n", taosd.Section.config.dbpassword)
	logger.Debug("dbName:\n", taosd.Section.config.dbName)
	//logger.Debug("tablePrefix:          %v\n", taosd.Section.config.tablePrefix)
	//logger.Debug("numOftables:          %v\n", taosd.Section.config.numOftables)
	//logger.Debug("numOfRecordsPerTable: %v\n", taosd.Section.config.numOfRecordsPerTable)
	//logger.Debug("numOfRecordsPerReq:   %v\n", taosd.Section.config.numOfRecordsPerReq)
	//logger.Debug("numOfThreads:         %v\n", taosd.Section.config.numOfThreads)
	//logger.Debug("startTimestamp:       %v[%v]\n", taosd.Section.config.startTimestamp, taosd.Section.config.startTs)
	logger.Debug("================================================\n")
}
func (taosd *TDengineDataSource) TAOSstrCmp(a string, b string) bool {
	//return if a locates before b in a dictrionary.
	for i := 0; i < len(a) && i < len(b); i++ {
		if int(a[i]-'0') > int(b[i]-'0') {
			return false
		} else if int(a[i]-'0') < int(b[i]-'0') {
			return true
		}
	}
	if len(a) > len(b) {
		return false
	} else {
		return true
	}
}

type TaosdClient struct {
	Db       *sql.DB
	Database string
	//Precision string
	querySQL   string
	resultQery []*map[string]interface{}
}

func NewInTaosdClient(section TDengineSection) (*TaosdClient, error) {
	db, err := sql.Open(section.config.taosDriverName, section.url+section.config.dbName) // taosd.Section.config.dbuser+":"+taosd.Section.config.dbpassword+"@/tcp("+taosd.Section.config.hostName+")/"+taosd.Section.config.dbName) //taosd.Section.config.dbuser+":"+taosd.Section.config.dbpassword+"@/tcp("+taosd.Section.config.hostName+")/"+taosd.Section.config.dbName)
	if err != nil {

		return nil, err
	}

	return &TaosdClient{
		Db:       db,
		Database: section.config.dbName,
		//Precision: section.Precision,
	}, nil
}

/*
// QueryData: || (|| endpoints...) (&& tags...)
func (taosd *TDengineDataSource)  QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data, inputs: %+v", inputs)

	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	if len(inputs) == 0 {
		return nil
	}

	query, opts := p.config.queryDataOptions(inputs)
	ret, err := fetchTagged(session, p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable to query data: ", err)
		return nil
	}

	return ret
}*/
//执行查询语句并返回查询结果
func (taosd *TDengineDataSource) taosdQuery(client *TaosdClient, querySQL QueryData) ([]*map[string]interface{}, error) {
	tbName := querySQL.Metric
	rows, e := client.Db.Query(querySQL.RawQuery)
	if e != nil {
		logger.Errorf("[%v]: failed to query TDengine: %v", tbName, e.Error())
		return nil, e
	}
	defer rows.Close()
	cols, e := rows.ColumnTypes()
	if e != nil {
		logger.Errorf("[%v]: unable to get column information: %v", tbName, e.Error())
		return nil, e
	}
	queryResponse := make([]*map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, 0, len(cols))
		for range cols {
			var v interface{}
			values = append(values, &v)
		}
		rows.Scan(values...)

		m := make(map[string]interface{})
		for i, col := range cols {
			name := strings.ToLower(col.Name())
			m[name] = *(values[i].(*interface{}))
		}
		queryResponse = append(queryResponse, &m)
		//alert := rule.getAlert(m)
		//alert.refresh(rule, m)
	}
	return queryResponse, nil
}
func (taosd *TDengineDataSource) taosdQueryOne(client *TaosdClient, querySQL QueryData) (resp *dataobj.TsdbQueryResponse, err error) {
	tbName := MetricnameEscape(querySQL.Metric)

	rows, e := client.Db.Query(querySQL.RawQuery)
	resp = &dataobj.TsdbQueryResponse{}
	if e != nil {
		logger.Errorf("[%v]: failed to query TDengine: %v", tbName, e.Error())
		return resp, e
	}
	defer rows.Close()
	fixed := make([]*dataobj.RRDData, 0)
	//	var value dataobj.RRDData
	cols, e := rows.ColumnTypes()
	if e != nil {
		logger.Errorf("[%v]: unable to get column information: %v", tbName, e.Error())
		return resp, fmt.Errorf("[%v]: unable to get column information: %v", tbName, e.Error())
	}
	for rows.Next() {
		values := make([]interface{}, 0, len(cols))
		for range cols {
			var v interface{}
			values = append(values, &v)
		}
		rows.Scan(values...)
		//var timestamp , flaotvalue
		var timestamp int64
		var floatvalue dataobj.JsonFloat
		timeTemplate1 := "2006-01-02 15:04:05" //常规类型
		//m := make(map[string]interface{})
		for i, col := range cols {
			name := strings.ToLower(col.Name())
			if name == "ts" {
				timestampString := (*(values[i].(*interface{}))).(string)
				timestampNumber := strings.Split(timestampString, ".")[0]
				tm2, _ := time.ParseInLocation(timeTemplate1, timestampNumber, time.Local)
				timestamp = int64(tm2.Unix())
				if taosd.Section.config.debugprt == 2 {
					fmt.Println(timestamp)
				}
			} else {
				if name == "value" {

					valuenumber := (*(values[i].(*interface{})))
					floatvalue, err = taosd.autochangevalue(valuenumber)
					if err != nil {
						logger.Errorf("autochangevalue：%s", err.Error())
						continue
					}
					//floatvalue = valuenumber.(float64)
					fixed = append(fixed, dataobj.NewRRDData(timestamp, float64(floatvalue)))
					//value.Value = (dataobj.JsonFloat)(doublevalue) //(*(values[i].(*interface{}))).(dataobj.JsonFloat)
				}
			}
			//m[name] = *(values[i].(*interface{}))
		}

	}
	resp.Values = fixed
	return resp, nil
}

//执行查询语句并返回查询结果
func (taosd *TDengineDataSource) taosdSQL(client *TaosdClient, SQL string) []*map[string]interface{} {
	rows, e := client.Db.Query(SQL)
	if e != nil {
		logger.Errorf("[%v]: failed to Excute TDengine SQL: %v", client.Database, e.Error())
		return nil
	}
	defer rows.Close()
	cols, e := rows.ColumnTypes()
	if e != nil {
		logger.Errorf("[%v]: unable to get column information: %v", client.Database, e.Error())
		return nil
	}
	queryResponse := make([]*map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, 0, len(cols))
		for range cols {
			var v interface{}
			values = append(values, &v)
		}
		rows.Scan(values...)

		m := make(map[string]interface{})
		for i, col := range cols {
			name := strings.ToLower(col.Name())
			m[name] = *(values[i].(*interface{}))
		}
		queryResponse = append(queryResponse, &m)
		//alert := rule.getAlert(m)
		//alert.refresh(rule, m)
	}
	return queryResponse
}

// show tag keys on n9e from metric where ...
// (exclude default endpoint tag)
func (taosd *TDengineDataSource) getTagMaps(client *TaosdClient, metric, database string, endpoints []string) []*dataobj.TagPair {
	tagkv := make([]*dataobj.TagPair, 0)
	//keys := make([]string, 0)
	//获取超级表结构，表名以metric创建的
	stabname := MetricnameEscape(metric)
	//通过表结构获取TAGS字段名数组
	tags := taosd.taosdGetTableTagDescribe(client.Db, stabname)
	//
	if tags == nil {
		return nil
	}
	taosdql := "select"
	for _, tagname := range tags {
		if tagname == "taghash" {
			continue
		}
		taosdql = taosdql + fmt.Sprintf(" %s,", tagname)
	}
	taosdql = taosdql[:len(taosdql)-len(",")]

	if len(endpoints) > 0 {
		endpointPart := ""
		for _, endpoint := range endpoints {
			endpointPart += fmt.Sprintf(" endpoint='%s' OR", endpoint)
		}
		endpointPart = endpointPart[:len(endpointPart)-len("OR")]
		taosdql = fmt.Sprintf("%s from  %s WHERE %s ", taosdql, stabname, endpointPart)
		if taosd.Section.config.debugprt == 2 {
			fmt.Printf(taosdql)
		}
		response := taosd.taosdSQL(client, taosdql)
		if response != nil {
			//resp := &dataobj.MetricResp{
			//	Metrics: make([]string, 0),
			//}
			for _, rowmap := range response {
				for tagKey, tagValue := range *rowmap {
					if tagValue != nil {
						pair := &dataobj.TagPair{
							Key:    tagKey,
							Values: []string{tagValue.(string)},
						}
						tagkv = append(tagkv, pair)
					}
				}
			}
		} else {
			logger.Warningf("query tag keys on taosd error.")
			return nil
		}
	}
	return tagkv
}

/*
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
   获取超级表结构，并将note 是 TAG的fild 取出来
*/
func (taosd *TDengineDataSource) taosdGetTableTagDescribe(db *sql.DB, stbname string) []string {
	taosdql := fmt.Sprintf("DESCRIBE %s;", stbname)
	//查询返回 按记录条->列名k 数据v保存为map数组
	//response := taosd.taosdSQL(c,taosdql)
	rows, e := db.Query(taosdql)
	if e != nil {
		logger.Errorf("[%v]: failed to Excute TDengine SQL: %v", e.Error(), taosdql)
		return nil
	}
	defer rows.Close()
	cols, e := rows.ColumnTypes()
	if e != nil {
		logger.Errorf("[%v]: unable to get column information: %v", e.Error(), taosdql)
		return nil
	}
	tagsNames := make([]string, 0)
	for rows.Next() {
		values := make([]interface{}, 0, len(cols))
		for range cols {
			var v interface{}
			values = append(values, &v)
		}
		rows.Scan(values...)

		m := make(map[string]interface{})
		for i, col := range cols {
			name := strings.ToLower(col.Name())
			value := *(values[i].(*interface{}))
			if value == "TAG" {
				tagname := *(values[0].(*interface{}))
				tagsNames = append(tagsNames, tagname.(string))
			}
			m[name] = value
		}
	}
	return tagsNames
}

//func (taosd *TDengineDataSource) taosdGetTagsDescribe( metric, endpoint string, client *TaosdClient) ([]string) {
func (taosd *TDengineDataSource) taosdGetTagsDescribe(client *TaosdClient, stbname string) []string {
	taosdql := fmt.Sprintf("DESCRIBE %s;", stbname)
	//查询返回 按记录条->列名k 数据v保存为map数组
	//response := taosd.taosdSQL(c,taosdql)
	rows, e := client.Db.Query(taosdql)
	if e != nil {
		logger.Errorf("[%v]: failed to Excute TDengine SQL: %v", e.Error(), taosdql)
		return nil
	}
	defer rows.Close()
	cols, e := rows.ColumnTypes()
	if e != nil {
		logger.Errorf("[%v]: unable to get column information: %v", e.Error(), taosdql)
		return nil
	}
	tagsNames := make([]string, 0)
	for rows.Next() {
		values := make([]interface{}, 0, len(cols))
		for range cols {
			var v interface{}
			values = append(values, &v)
		}
		rows.Scan(values...)

		m := make(map[string]interface{})
		for i, col := range cols {
			name := strings.ToLower(col.Name())
			value := *(values[i].(*interface{}))
			if value == "TAG" {
				tagname := *(values[0].(*interface{}))
				tagsNames = append(tagsNames, tagname.(string))
			}
			m[name] = value
		}
	}
	return tagsNames
}

// tsdb instance
func (taosd *TDengineDataSource) GetInstance(metric, endpoint string, tags map[string]string) []string {
	return []string{taosd.Section.config.daemonName}
}

func convertToConfig(configmap map[interface{}]interface{}) (config TaosConfiguration, err error) {

	//var config TaosConfiguration
	for k, v := range configmap {
		//if taosd.Section.config.debugprt == 2 {
		fmt.Println(k, v)
		//}
		switch k.(string) {
		case "daemonIP":
			config.daemonIP = v.(string) //127.0.0.1
		case "daemonName":
			config.daemonName = v.(string) //localhost
		case "serverPort":
			config.serverPort = v.(int) //6030
		case "apiport":
			config.apiport = v.(int) //6020
		case "dbuser":
			config.dbuser = v.(string) //root
		case "dbpassword":
			config.dbpassword = v.(string) //taosdata
		case "dbName": //"n9e"
			config.dbName = v.(string)
		case "taosDriverName": //"taosSql"
			config.taosDriverName = v.(string)
		case "inputworkers": //5
			config.inputworkers = v.(int)
		case "insertsqlworkers": //= 5
			config.insertsqlworkers = v.(int)
		//supTblName           string
		//tablePrefix          string
		//numOftables          int
		//numOfRecordsPerTable int
		//numOfRecordsPerReq   int
		//numOfThreads         int
		case "insertbatchSize": //= 100
			config.insertbatchSize = v.(int)
		case "inputbuffersize": //= 100
			config.inputbuffersize = v.(int)
		/*
			case "useindex":
				config.useindex = v.(bool)
			case "index":
				for kk, vv := range v.(map[interface{}]interface{}) {
					if kk == "activeDuration" {
						config.Index.ActiveDuration = int64(vv.(int))
					} else if kk == "rebuildInterval" {
						config.Index.RebuildInterval = int64(vv.(int))
					} else if kk == "hbsMod" {
						config.Index.HbsMod = vv.(string)
					}
				}
		*/
		/*
			case "rpcClient":
				for kk, vv := range v.(map[interface{}]interface{}) {
					if kk == "callTimeout" {
						config.RpcClient.CallTimeout = vv.(int)
					} else if kk == "connTimeout" {
						config.RpcClient.ConnTimeout = vv.(int)
					} else if kk == "maxConns" {
						config.RpcClient.MaxConns = vv.(int)
					} else if kk == "maxIdle" {
						config.RpcClient.MaxIdle = vv.(int)
					}
				}
		*/
		case "debugprt": //= 2 //if 0 not print, if 1 print the sql,
			config.debugprt = v.(int)
		case "taglen": //= 128
			config.taglen = v.(int)
		case "taglimit": //= 1024
			config.taglimit = v.(int)
		case "tagnumlimit": //= 128
			config.tagnumlimit = v.(int)
		//	tablepervnode int
		//startTimestamp string `mapstructure:"startTimestamp"`
		//startTs        int64  `mapstructure:"startTs"`

		case "keep": //=365*5 //5 years数据保留期
			config.keep = v.(int)
		case "days": //=30    //一个月1个文件
			config.days = v.(int)
		case "blocks": //=4     //内存块
			config.blocks = v.(int)
		}
	}
	return config, nil
}
func YamlParse(bs []byte, taosdconfig *TDengineSection) (err error) {
	//conf := new(module.Yaml)
	//var confyaml ConfYaml
	//var taosdconfig TDengineSection
	//buffer, err := ioutil.ReadFile("transfer - 副本.yml")
	//if err != nil {
	//	fmt.Println("读取配置文件失败, 异常信息 : ", err)
	//}
	buffer := bs
	yamlmap := make(map[string]interface{})
	err = yaml.Unmarshal(buffer, yamlmap)
	if err != nil {
		fmt.Println("Unmarshal ", err)
	}
	//fmt.Println("conf ", yamlmap)
	for k, v := range yamlmap {
		if k == "backend" {
			//taosdmap := make(map[interface{}]interface{})
			taosdmap := v.(map[interface{}]interface{})
			//if taosd.Section.config.debugprt == 2 {
			fmt.Println("backend ", taosdmap)
			//}
			for kk, vv := range taosdmap {
				if kk.(string) == "taosd" {
					//	if taosd.Section.config.debugprt == 2 {
					fmt.Println("taosd ", vv)
					//	}
					taosdmap1 := vv.(map[interface{}]interface{})
					if err := mapstructure.Decode(taosdmap1, &taosdconfig); err != nil {
						fmt.Println(err)
					}
					//	if taosd.Section.config.debugprt == 2 {
					fmt.Printf("map2struct后得到的 struct 内容为:%v", taosdconfig)
					//	}
					for kkk, vvv := range taosdmap1 {

						fmt.Println("\r\n"+kkk.(string), vvv)
						switch kkk.(string) {
						case "config":
							configmap := vvv.(map[interface{}]interface{})
							//			if taosd.Section.config.debugprt == 2 {
							fmt.Println("\r\n"+"configmap", configmap)
							//			}
							var config TaosConfiguration

							if config, err = convertToConfig(configmap); err != nil {

								//	if err = mapstructure.Decode(configmap, &config); err != nil {
								//if err = yaml.Unmarshal(configmap, &config); err != nil {
								fmt.Println(err)
							}
							//if taosd.Section.config.debugprt == 2 {
							fmt.Printf("\r\nconvertToConfig后得到的 struct 内容为:%v", config)
							//	}
							taosdconfig.config = config
							//	if taosd.Section.config.debugprt == 2 {
							fmt.Printf("\r\ntaosdconfig struct 内容为:%v", taosdconfig)
							//	}
						}
					}
				}
			}
		}
	}
	return err
}

/*
func selectTest(dbName string, tbPrefix string, supTblName string) {
	db, err := sql.Open(taosDriverName, url)
	if err != nil {
		fmt.Println("Open database error: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// select sql 1
	limit := 3
	offset := 0
	sqlStr := "select * from " + dbName + "." + supTblName + " limit " + strconv.Itoa(limit) + " offset " + strconv.Itoa(offset)
	rows, err := db.Query(sqlStr)
	checkErr(err, sqlStr)

	defer rows.Close()
	logger.Debug("query sql: %s\n", sqlStr)
	for rows.Next() {
		var (
			ts       string
			current  float32
			voltage  int
			phase    float32
			location string
			groupid  int
		)
		err := rows.Scan(&ts, &current, &voltage, &phase, &location, &groupid)
		if err != nil {
			checkErr(err, "rows scan fail")
		}

		logger.Debug("ts:%s\t current:%f\t voltage:%d\t phase:%f\t location:%s\t groupid:%d\n", ts, current, voltage, phase, location, groupid)
	}
	// check iteration error
	if rows.Err() != nil {
		checkErr(err, "rows next iteration error")
	}

	// select sql 2
	sqlStr = "select avg(voltage), min(voltage), max(voltage) from " + dbName + "." + tbPrefix + strconv.Itoa(rand.Int()%taosd.Section.config.numOftables)
	rows, err = db.Query(sqlStr)
	checkErr(err, sqlStr)

	defer rows.Close()
	logger.Debug("\nquery sql: %s\n", sqlStr)
	for rows.Next() {
		var (
			voltageAvg float32
			voltageMin int
			voltageMax int
		)
		err := rows.Scan(&voltageAvg, &voltageMin, &voltageMax)
		if err != nil {
			checkErr(err, "rows scan fail")
		}

		logger.Debug("avg(voltage):%f\t min(voltage):%d\t max(voltage):%d\n", voltageAvg, voltageMin, voltageMax)
	}
	// check iteration error
	if rows.Err() != nil {
		checkErr(err, "rows next iteration error")
	}

	// select sql 3
	sqlStr = "select last(*) from " + dbName + "." + supTblName
	rows, err = db.Query(sqlStr)
	checkErr(err, sqlStr)

	defer rows.Close()
	logger.Debug("\nquery sql: %s\n", sqlStr)
	for rows.Next() {
		var (
			lastTs      string
			lastCurrent float32
			lastVoltage int
			lastPhase   float32
		)
		err := rows.Scan(&lastTs, &lastCurrent, &lastVoltage, &lastPhase)
		if err != nil {
			checkErr(err, "rows scan fail")
		}

		logger.Debug("last(ts):%s\t last(current):%f\t last(voltage):%d\t last(phase):%f\n", lastTs, lastCurrent, lastVoltage, lastPhase)
	}
	// check iteration error
	if rows.Err() != nil {
		checkErr(err, "rows next iteration error")
	}
}
*/

/*
func main() {
	printAllArgs()
	logger.Debug("Please press enter key to continue....\n")
	fmt.Scanln()

	url = "root:taosdata@/tcp(" + taosd.Section.config.hostName + ":" + strconv.Itoa(taosd.Section.config.serverPort) + ")/"
	//url = fmt.Sprintf("%s:%s@/tcp(%s:%d)/%s?interpolateParams=true", taosd.Section.config.user, taosd.Section.config.password, taosd.Section.config.hostName, taosd.Section.config.serverPort, taosd.Section.config.dbName)
	// open connect to taos server
	//db, err := sql.Open(taosDriverName, url)
	//if err != nil {
	//  fmt.Println("Open database error: %s\n", err)
	//  os.Exit(1)
	//}
	//defer db.Close()
	rand.Seed(time.Now().Unix())

	createDatabase(taosd.Section.config.dbName, taosd.Section.config.supTblName)
	logger.Debug("======== create database success! ========\n\n")

	//create_table(db, stblName)
	multiThreadCreateTable(taosd.Section.config.numOfThreads, taosd.Section.config.numOftables, taosd.Section.config.dbName, taosd.Section.config.tablePrefix)
	logger.Debug("======== create super table and child tables success! ========\n\n")

	//insert_data(db, demot)
	multiThreadInsertData(taosd.Section.config.numOfThreads, taosd.Section.config.numOftables, taosd.Section.config.dbName, taosd.Section.config.tablePrefix)
	logger.Debug("======== insert data into child tables success! ========\n\n")

	//select_data(db, demot)
	selectTest(taosd.Section.config.dbName, taosd.Section.config.tablePrefix, taosd.Section.config.supTblName)
	logger.Debug("======== select data success!  ========\n\n")

	logger.Debug("======== end demo ========\n")
}

func createDatabase(dbName string, supTblName string) {
	db, err := sql.Open(taosDriverName, url)
	if err != nil {
		fmt.Println("Open database error: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// drop database if exists
	sqlStr := "drop database if exists " + dbName
	_, err = db.Exec(sqlStr)
	checkErr(err, sqlStr)

	time.Sleep(time.Second)

	// create database  databasename KEEP 365 DAYS 10 BLOCKS 4
	sqlStr = "create database " + dbName + " keep " + strconv.Itoa(taosd.Section.config.keep) + " days " + strconv.Itoa(taosd.Section.config.days)
	_, err = db.Exec(sqlStr)
	checkErr(err, sqlStr)

	// use database
	//sqlStr = "use " + dbName
	//_, err = db.Exec(sqlStr)
	//checkErr(err, sqlStr)

	sqlStr = "create table if not exists " + dbName + "." + supTblName + " (ts timestamp, current float, voltage int, phase float) tags(location binary(64), groupId int);"
	_, err = db.Exec(sqlStr)
	checkErr(err, sqlStr)
}

func multiThreadCreateTable(threads int, ntables int, dbName string, tablePrefix string) {
	st := time.Now().UnixNano()

	if threads < 1 {
		threads = 1
	}

	a := ntables / threads
	if a < 1 {
		threads = ntables
		a = 1
	}

	b := ntables % threads

	last := 0
	endTblId := 0
	wg := sync.WaitGroup{}
	for i := 0; i < threads; i++ {
		startTblId := last
		if i < b {
			endTblId = last + a
		} else {
			endTblId = last + a - 1
		}
		last = endTblId + 1
		wg.Add(1)
		go createTable(dbName, tablePrefix, startTblId, endTblId, &wg)
	}
	wg.Wait()

	et := time.Now().UnixNano()
	logger.Debug("create tables spent duration: %6.6fs\n", (float32(et-st))/1e9)
}

func createTable(dbName string, childTblPrefix string, startTblId int, endTblId int, wg *sync.WaitGroup) {
	//logger.Debug("subThread[%d]: create table from %d to %d \n", unix.Gettid(), startTblId, endTblId)
	// windows.GetCurrentThreadId()

	db, err := sql.Open(taosDriverName, url)
	if err != nil {
		fmt.Println("Open database error: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	for i := startTblId; i <= endTblId; i++ {
		sqlStr := "create table if not exists " + dbName + "." + childTblPrefix + strconv.Itoa(i) + " using " + dbName + ".meters tags('" + locations[i%maxLocationSize] + "', " + strconv.Itoa(i) + ");"
		//logger.Debug("sqlStr:               %v\n", sqlStr)
		_, err = db.Exec(sqlStr)
		checkErr(err, sqlStr)
	}
	wg.Done()
	runtime.Goexit()
}

func generateRowData(ts int64) string {
	voltage := rand.Int() % 1000
	current := 200 + rand.Float32()
	phase := rand.Float32()
	values := "( " + strconv.FormatInt(ts, 10) + ", " + strconv.FormatFloat(float64(current), 'f', 6, 64) + ", " + strconv.Itoa(voltage) + ", " + strconv.FormatFloat(float64(phase), 'f', 6, 64) + " ) "
	return values
}
func insertData(dbName string, childTblPrefix string, startTblId int, endTblId int, wg *sync.WaitGroup) {
	//logger.Debug("subThread[%d]: insert data to table from %d to %d \n", unix.Gettid(), startTblId, endTblId)
	// windows.GetCurrentThreadId()

	db, err := sql.Open(taosDriverName, url)
	if err != nil {
		fmt.Println("Open database error: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	tmpTs := taosd.Section.config.startTs
	//rand.New(rand.NewSource(time.Now().UnixNano()))
	for tID := startTblId; tID <= endTblId; tID++ {
		totalNum := 0
		for {
			sqlStr := "insert into " + dbName + "." + childTblPrefix + strconv.Itoa(tID) + " values "
			currRowNum := 0
			for {
				tmpTs += 1000
				valuesOfRow := generateRowData(tmpTs)
				currRowNum += 1
				totalNum += 1

				sqlStr = fmt.Sprintf("%s %s", sqlStr, valuesOfRow)

				if currRowNum >= taosd.Section.config.numOfRecordsPerReq || totalNum >= taosd.Section.config.numOfRecordsPerTable {
					break
				}
			}

			res, err := db.Exec(sqlStr)
			checkErr(err, sqlStr)

			count, err := res.RowsAffected()
			checkErr(err, "rows affected")

			if count != int64(currRowNum) {
				logger.Debug("insert data, expect affected:%d, actual:%d\n", currRowNum, count)
				os.Exit(1)
			}

			if totalNum >= taosd.Section.config.numOfRecordsPerTable {
				break
			}
		}
	}

	wg.Done()
	runtime.Goexit()
}
func multiThreadInsertData(threads int, ntables int, dbName string, tablePrefix string) {
	st := time.Now().UnixNano()

	if threads < 1 {
		threads = 1
	}

	a := ntables / threads
	if a < 1 {
		threads = ntables
		a = 1
	}

	b := ntables % threads

	last := 0
	endTblId := 0
	wg := sync.WaitGroup{}
	for i := 0; i < threads; i++ {
		startTblId := last
		if i < b {
			endTblId = last + a
		} else {
			endTblId = last + a - 1
		}
		last = endTblId + 1
		wg.Add(1)
		go insertData(dbName, tablePrefix, startTblId, endTblId, &wg)
	}
	wg.Wait()

	et := time.Now().UnixNano()
	logger.Debug("insert data spent duration: %6.6fs\n", (float32(et-st))/1e9)
}
func selectTest(dbName string, tbPrefix string, supTblName string) {
	db, err := sql.Open(taosDriverName, url)
	if err != nil {
		fmt.Println("Open database error: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// select sql 1
	limit := 3
	offset := 0
	sqlStr := "select * from " + dbName + "." + supTblName + " limit " + strconv.Itoa(limit) + " offset " + strconv.Itoa(offset)
	rows, err := db.Query(sqlStr)
	checkErr(err, sqlStr)

	defer rows.Close()
	logger.Debug("query sql: %s\n", sqlStr)
	for rows.Next() {
		var (
			ts       string
			current  float32
			voltage  int
			phase    float32
			location string
			groupid  int
		)
		err := rows.Scan(&ts, &current, &voltage, &phase, &location, &groupid)
		if err != nil {
			checkErr(err, "rows scan fail")
		}

		logger.Debug("ts:%s\t current:%f\t voltage:%d\t phase:%f\t location:%s\t groupid:%d\n", ts, current, voltage, phase, location, groupid)
	}
	// check iteration error
	if rows.Err() != nil {
		checkErr(err, "rows next iteration error")
	}

	// select sql 2
	sqlStr = "select avg(voltage), min(voltage), max(voltage) from " + dbName + "." + tbPrefix + strconv.Itoa(rand.Int()%taosd.Section.config.numOftables)
	rows, err = db.Query(sqlStr)
	checkErr(err, sqlStr)

	defer rows.Close()
	logger.Debug("\nquery sql: %s\n", sqlStr)
	for rows.Next() {
		var (
			voltageAvg float32
			voltageMin int
			voltageMax int
		)
		err := rows.Scan(&voltageAvg, &voltageMin, &voltageMax)
		if err != nil {
			checkErr(err, "rows scan fail")
		}

		logger.Debug("avg(voltage):%f\t min(voltage):%d\t max(voltage):%d\n", voltageAvg, voltageMin, voltageMax)
	}
	// check iteration error
	if rows.Err() != nil {
		checkErr(err, "rows next iteration error")
	}

	// select sql 3
	sqlStr = "select last(*) from " + dbName + "." + supTblName
	rows, err = db.Query(sqlStr)
	checkErr(err, sqlStr)

	defer rows.Close()
	logger.Debug("\nquery sql: %s\n", sqlStr)
	for rows.Next() {
		var (
			lastTs      string
			lastCurrent float32
			lastVoltage int
			lastPhase   float32
		)
		err := rows.Scan(&lastTs, &lastCurrent, &lastVoltage, &lastPhase)
		if err != nil {
			checkErr(err, "rows scan fail")
		}

		logger.Debug("last(ts):%s\t last(current):%f\t last(voltage):%d\t last(phase):%f\n", lastTs, lastCurrent, lastVoltage, lastPhase)
	}
	// check iteration error
	if rows.Err() != nil {
		checkErr(err, "rows next iteration error")
	}
}
func checkErr(err error, prompt string) {
	if err != nil {
		logger.Debug("%s\n", prompt)
		panic(err)
	}
}
*/
