# transfer
n9e https://github.com/didi/nightingale
TDengine https://github.com/taosdata/TDengine
<pre><code>
modify:
transfer/backend/init.go /
transfer/config/config.go /

add:

transfer/backend/taosd/

modify:

nginx.conf
modify index ---> transfer

judge.yml

query:
  indexMod: transfer
  connTimeout: 1000
  callTimeout: 2000
  indexCallTimeout: 2000
 
redis:
  addrs: 
    - 127.0.0.1:6379
  pass: ""
  # timeout:
  #   conn: 500
  #   read: 3000
  #   write: 3000

logger:
  dir: logs/judge
  level: INFO
  keepHours: 24
  
monapi.yml
---
tokens:
  - monapi-internal-third-module-pass-fjsdi

indexMod: transfer

logger:
  dir: logs/monapi
  level: INFO
  keepHours: 24

alarmEnabled: true

region:
  - default

# clean history event
cleaner:
  # retention days
  days: 100
  # number of events deleted per time
  batch: 100

# read alert from redis
redis:
  addr: 127.0.0.1:6379
  pass: ""
  # timeout:
  #   conn: 500
  #   read: 3000
  #   write: 3000
i18n:
  lang: zh

notify:
  p1: ["voice", "sms", "mail", "im"]
  p2: ["sms", "mail", "im"]
  p3: ["mail", "im"]

# addresses accessible using browser
link:
  stra: http://n9e.com/mon/strategy/%v
  event: http://n9e.com/mon/history/his/%v
  claim: http://n9e.com/mon/history/cur/%v

http:
  mode: release
  cookieDomain: ""
  cookieName: ecmc-sid

transfer.yml
backend:
  datasource: "taosd"
  taosd:     #for TDengine 
    enabled: true
    name: "taosd"
    config:  
      daemonIP: "192.168.0.25"
      daemonName: "192.168.0.25"
      serverPort: 6030
      apiport: 6020
      dbuser: "root"
      dbpassword: "taosdata"
      dbName: "n9e"
      taosDriverName: "taosSql"
      inputworkers: 20
      insertsqlworkers: 20 #Adjust according to PC quantity and index quantity
      insertbatchSize: 500 #Adjust according to PC quantity and index quantity
      inputbuffersize: 500 #Adjust according to PC quantity and index quantity
      queryworkers: 20 #Adjust according to PC quantity and index quantity
      querydatachs: 100 #Adjust according to PC quantity and index quantity
      insertTimeout: 2 #Adjust according to PC quantity and index quantity
      callTimeout: 1000 #Adjust according to PC quantity and index quantity
      debugprt: 0 #if 0 not print, if 2 print the sql,
      taglen: 128
      taglimit: 1024
      tagnumlimit: 128
      keep: 3650 #10 years数据保留期
      days: 30    #一个月1个文件
      blocks: 4     #内存块
      url: ""
      tdurl: ""
      tagstr: ""
  m3db:
    enabled: false
    name: "m3db"
    namespace: "default"
    seriesLimit: 0
    docsLimit: 0
    daysLimit: 7                               # max query time
    # https://m3db.github.io/m3/m3db/architecture/consistencylevels/
    writeConsistencyLevel: "majority"          # one|majority|all
    readConsistencyLevel: "unstrict_majority"  # one|unstrict_majority|majority|all
    config:
      service:
        # KV environment, zone, and service from which to write/read KV data (placement
        # and configuration). Leave these as the default values unless you know what
        # you're doing.
        env: default_env
        zone: embedded
        service: m3db
        etcdClusters:
          - zone: embedded
            endpoints:
              - 127.0.0.1:2379
            tls:
              caCrtPath: /etc/etcd/certs/ca.pem
              crtPath: /etc/etcd/certs/etcd-client.pem
              keyPath: /etc/etcd/certs/etcd-client-key.pem
  tsdb:
    enabled: false
    name: "tsdb"
    cluster:
      tsdb01: 127.0.0.1:8011
  influxdb:
    enabled: false
    username: "influx"
    password: "admin123"
    precision: "s"
    database: "n9e"
    address: "http://127.0.0.1:8086"
  opentsdb:
    enabled: false
    address: "127.0.0.1:4242"
  kafka:
    enabled: false
    brokersPeers: "192.168.1.1:9092,192.168.1.2:9092"
    topic: "n9e"
http:
  mode: release
  cookieDomain: ""
  cookieName: ecmc-sid
  
logger:
  dir: logs/transfer
  level: DEBUG
  keepHours: 24
  
  
taos> show stables;
              name              |      created_time       | columns |  tags  |   tables    |
============================================================================================
 n9e_judge_event_recover        | 2021-02-02 12:02:43.405 |       2 |      2 |           1 |
 cpu_guest                      | 2021-01-27 20:36:27.234 |       2 |      2 |           3 |
 cpu_switches                   | 2021-01-27 20:36:27.240 |       2 |      2 |           3 |
 redis_db_keys                  | 2021-03-12 13:55:09.519 |       2 |      6 |          64 |
 redis_blocked_clients          | 2021-03-12 13:55:09.511 |       2 |      5 |           3 |
 redis_commands_duration_sec... | 2021-03-23 10:18:45.831 |       2 |      6 |          36 |
 redis_config_maxmemory         | 2021-03-12 13:55:09.865 |       2 |      5 |           3 |
 sys_ntp_offset_ms              | 2021-01-27 20:36:35.325 |       2 |      2 |          18 |
 redis_mem_fragmentation_ratio  | 2021-03-12 13:55:09.714 |       2 |      5 |           3 |
 n9e_judge_redis_push           | 2021-01-30 20:37:13.418 |       2 |      2 |           1 |
 net_in_errs                    | 2021-01-27 20:36:27.621 |       2 |      3 |          21 |
 net_out_percent                | 2021-01-27 20:36:27.642 |       2 |      3 |          21 |
 redis_cpu_user_seconds_total   | 2021-03-23 10:18:46.066 |       2 |      5 |           3 |
 cpu_loadavg_1                  | 2021-01-27 20:36:27.253 |       2 |      2 |           3 |
 mysqld_exporter_build_info     | 2021-03-12 13:55:09.464 |       2 |     10 |           8 |
 redis_aof_rewrite_scheduled    | 2021-03-12 13:55:09.952 |       2 |      5 |           3 |
 disk_bytes_free                | 2021-01-27 20:36:27.515 |       2 |      4 |          86 |
 n9e_judge_query_index_err      | 2021-01-27 22:53:33.412 |       2 |      2 |           1 |
 disk_io_util                   | 2021-01-27 20:36:27.313 |       2 |      3 |          40 |
 redis_pubsub_patterns          | 2021-03-12 13:55:09.876 |       2 |      5 |           3 |
 promhttp_metric_handler_req... | 2021-03-23 10:18:46.170 |       2 |      7 |          15 |
 disk_inodes_used               | 2021-01-27 20:36:27.545 |       2 |      3 |           3 |
 process_start_time_seconds     | 2021-03-12 13:55:09.429 |       2 |      6 |           7 |
 snmp_tcp_passiveopens          | 2021-01-27 20:36:27.407 |       2 |      2 |           3 |
 redis_connected_clients        | 2021-03-12 13:55:09.747 |       2 |      5 |           3 |
 mem_bytes_cached               | 2021-01-27 20:36:27.144 |       2 |      2 |           3 |
 redis_exporter_scrape_durat... | 2021-03-23 10:18:45.905 |       2 |      5 |           3 |
 redis_replication_backlog_b... | 2021-03-12 13:55:09.617 |       2 |      5 |           3 |
 redis_slowlog_length           | 2021-03-12 13:55:09.503 |       2 |      5 |           3 |
 sys_net_netfilter_nf_conntr... | 2021-03-12 13:53:39.460 |       2 |      2 |           1 |
 snmp_tcp_outsegs               | 2021-01-27 20:36:27.386 |       2 |      2 |           3 |
 n9e_judge_query_data           | 2021-01-27 22:52:33.433 |       2 |      2 |           1 |
 process_virtual_memory_bytes   | 2021-03-12 13:55:09.448 |       2 |      6 |           7 |
 disk_bytes_used                | 2021-01-27 20:36:27.521 |       2 |      4 |          86 |
 mem_bytes_free                 | 2021-01-27 20:36:27.121 |       2 |      2 |          21 |
 n9e_judge_stra_get_err         | 2021-01-27 20:36:26.962 |       2 |      2 |           2 |
 redis_expired_keys_total       | 2021-03-23 10:18:46.071 |       2 |      5 |           2 |
 redis_commands_total           | 2021-03-23 10:18:45.967 |       2 |      6 |          36 |
 snmp_tcp_activeopens           | 2021-01-27 20:36:27.455 |       2 |      2 |           3 |
 snmp_tcp_incsumerrors          | 2021-01-27 20:36:27.396 |       2 |      2 |           3 |
 process_resident_memory_bytes  | 2021-03-12 13:55:09.422 |       2 |      6 |           7 |
 n9e_judge_stra_common          | 2021-01-27 22:52:23.406 |       2 |      2 |           1 |
 process_open_fds               | 2021-03-12 13:55:09.453 |       2 |      6 |           7 |
 sys_net_tcp_ip4_con_reset      | 2021-01-27 20:36:35.304 |       2 |      2 |          18 |
 net_in_percent                 | 2021-01-27 20:36:27.634 |       2 |      3 |          21 |
 n9e_transfer_points_in         | 2021-01-27 20:36:36.376 |       2 |      2 |           2 |
 snmp_udp_noports               | 2021-01-27 20:36:27.466 |       2 |      2 |           3 |
 snmp_tcp_currestab             | 2021-01-27 20:36:27.424 |       2 |      2 |           3 |
 redis_rdb_last_bgsave_status   | 2021-03-12 13:55:09.932 |       2 |      5 |           3 |
 redis_replica_partial_resyn... | 2021-03-12 13:55:09.921 |       2 |      5 |           3 |
 redis_config_maxclients        | 2021-03-12 13:55:09.685 |       2 |      5 |           3 |
 redis_cluster_enabled          | 2021-03-12 13:55:09.491 |       2 |      5 |           3 |
 snmp_udp_inerrors              | 2021-01-27 20:36:27.473 |       2 |      2 |           3 |
 mem_bytes_used                 | 2021-01-27 20:36:27.116 |       2 |      2 |          21 |
 redis_commands_processed_total | 2021-03-23 10:18:45.927 |       2 |      5 |           3 |
 sys_uptime_duration            | 2021-02-02 11:28:19.198 |       2 |      2 |           9 |
 redis_master_repl_offset       | 2021-03-12 13:55:09.638 |       2 |      5 |           3 |
 redis_aof_last_write_status    | 2021-03-12 13:55:09.781 |       2 |      5 |           3 |
 cpu_user                       | 2021-01-27 20:36:27.189 |       2 |      2 |          21 |
 snmp_tcp_retranssegs           | 2021-01-27 20:36:27.380 |       2 |      2 |           3 |
 redis_rejected_connections_... | 2021-03-23 10:18:46.002 |       2 |      5 |           3 |
 redis_client_longest_output... | 2021-03-12 13:55:09.583 |       2 |      5 |           3 |
 cpu_irq                        | 2021-01-27 20:36:27.213 |       2 |      2 |          21 |
 sys_ps_entity_total            | 2021-01-27 20:36:27.503 |       2 |      2 |           3 |
 cpu_softirq                    | 2021-01-27 20:36:27.219 |       2 |      2 |           3 |
 redis_up                       | 2021-03-12 13:55:09.894 |       2 |      5 |           3 |
 n9e_judge_stra_nodata          | 2021-01-27 22:52:23.422 |       2 |      2 |           1 |
 cpu_nice                       | 2021-01-27 20:36:27.194 |       2 |      2 |           3 |
 redis_net_input_bytes_total    | 2021-03-23 10:18:45.947 |       2 |      5 |           3 |
 n9e_transfer_tag_qp10s         | 2021-01-27 22:45:43.424 |       2 |      2 |           1 |
 redis_rdb_changes_since_las... | 2021-03-12 13:55:09.882 |       2 |      5 |           3 |
 disk_io_read_msec              | 2021-01-27 20:36:35.228 |       2 |      3 |          32 |
 net_out_dropped                | 2021-01-27 20:36:27.597 |       2 |      3 |          21 |
 n9e_transfer_taosdb_inputno... | 2021-01-27 20:36:36.389 |       2 |      2 |           1 |
 net_in_bits_total              | 2021-01-27 20:36:27.664 |       2 |      2 |          21 |
 redis_latest_fork_seconds      | 2021-03-12 13:55:09.595 |       2 |      5 |           3 |
 sys_net_netfilter_nf_conntr... | 2021-03-12 13:53:39.466 |       2 |      2 |           1 |
 redis_replica_resyncs_full     | 2021-03-12 13:55:09.938 |       2 |      5 |           3 |
 redis_repl_backlog_is_active   | 2021-03-12 13:55:09.753 |       2 |      5 |           3 |
 redis_process_id               | 2021-03-12 13:55:09.650 |       2 |      5 |           3 |
 net_out_bits_total             | 2021-01-27 20:36:27.671 |       2 |      2 |          21 |
 process_cpu_seconds_total      | 2021-03-23 10:18:45.910 |       2 |      6 |           7 |
 redis_rdb_bgsave_in_progress   | 2021-03-12 13:55:09.910 |       2 |      5 |           3 |
 disk_cap_bytes_total           | 2021-01-27 20:36:27.556 |       2 |      2 |          21 |
 redis_aof_last_bgrewrite_st... | 2021-03-12 13:55:09.947 |       2 |      5 |           3 |
 redis_exporter_last_scrape_... | 2021-03-12 13:55:09.497 |       2 |      5 |           3 |
 redis_db_avg_ttl_seconds       | 2021-03-12 13:55:09.480 |       2 |      6 |           4 |
 sys_fs_files_max               | 2021-01-27 20:36:27.352 |       2 |      2 |           3 |
 redis_keyspace_misses_total    | 2021-03-23 10:18:46.127 |       2 |      5 |           3 |
 mysql_exporter_scrapes_total   | 2021-03-23 10:18:46.181 |       2 |      6 |           4 |
 n9e_judge_push_in              | 2021-01-27 22:52:33.404 |       2 |      2 |           1 |
 snmp_tcp_rtoalgorithm          | 2021-01-27 20:36:27.444 |       2 |      2 |           3 |
 n9e_judge_redis_push_failed    | 2021-03-24 03:42:40.858 |       2 |      2 |           1 |
 disk_io_write_request          | 2021-01-27 20:36:27.275 |       2 |      3 |          40 |
 disk_inodes_total              | 2021-01-27 20:36:27.533 |       2 |      3 |           3 |
 redis_connected_slaves         | 2021-03-12 13:55:09.708 |       2 |      5 |           3 |
 n9e_index_query_tag_miss       | 2021-01-27 22:46:53.301 |       2 |      2 |           1 |
 redis_memory_max_bytes         | 2021-03-12 13:55:09.731 |       2 |      5 |           3 |
 snmp_tcp_rtomax                | 2021-01-27 20:36:27.375 |       2 |      2 |           3 |
 redis_exporter_scrape_durat... | 2021-03-23 10:18:45.900 |       2 |      5 |           3 |
 n9e_tsdb_index_delete          | 2021-01-27 20:38:56.839 |       2 |      2 |           1 |
 n9e_prober_collectrule_count   | 2021-01-27 20:36:27.074 |       2 |      2 |           1 |
 cpu_loadavg_15                 | 2021-01-27 20:36:27.264 |       2 |      2 |           3 |
 agent_metric_cache_size        | 2021-01-27 20:36:27.033 |       2 |      2 |           3 |
 redis_rdb_last_save_timesta... | 2021-03-12 13:55:09.486 |       2 |      5 |           3 |
 n9e_judge_query_index          | 2021-01-27 22:52:33.409 |       2 |      2 |           1 |
 n9e_prober_collectrule_common  | 2021-01-28 15:34:43.537 |       2 |      2 |           1 |
 redis_net_output_bytes_total   | 2021-03-23 10:18:45.882 |       2 |      5 |           3 |
 n9e_prober_collectrule_get_err | 2021-01-27 20:36:27.068 |       2 |      2 |           1 |
 redis_db_keys_expiring         | 2021-03-12 13:55:09.796 |       2 |      6 |          64 |
 n9e_judge_query_data_transf... | 2021-02-21 00:04:20.863 |       2 |      2 |           1 |
 n9e_transfer_counter_qp10s     | 2021-01-27 22:45:43.381 |       2 |      2 |           1 |
 redis_instance_info            | 2021-03-12 13:55:09.725 |       2 |     11 |          53 |
 redis_keyspace_hits_total      | 2021-03-23 10:18:45.867 |       2 |      5 |           3 |
 snmp_tcp_estabresets           | 2021-01-27 20:36:27.417 |       2 |      2 |           3 |
 redis_memory_used_bytes        | 2021-03-12 13:55:09.664 |       2 |      5 |           3 |
 redis_exporter_last_scrape_... | 2021-03-12 13:55:09.692 |       2 |      5 |           3 |
 snmp_udp_sndbuferrors          | 2021-01-27 20:36:27.489 |       2 |      2 |           3 |
 net_bandwidth_mbits_total      | 2021-01-27 20:36:27.656 |       2 |      2 |          21 |
 snmp_tcp_attemptfails          | 2021-01-27 20:36:27.412 |       2 |      2 |           3 |
 mem_bytes_buffers              | 2021-01-27 20:36:27.138 |       2 |      2 |           3 |
 redis_exporter_scrapes_total   | 2021-03-23 10:18:46.076 |       2 |      5 |           3 |
 sys_net_tcp_ip4_con_establi... | 2021-01-27 20:36:35.316 |       2 |      2 |          18 |
 disk_bytes_used_percent        | 2021-01-27 20:36:27.527 |       2 |      4 |          86 |
 redis_cpu_sys_seconds_total    | 2021-03-23 10:18:46.015 |       2 |      5 |           3 |
 n9e_transfer_judge_get_err     | 2021-01-27 20:36:36.397 |       2 |      2 |           2 |
 n9e_index_xclude_miss          | 2021-01-27 22:52:33.304 |       2 |      2 |           1 |
 redis_start_time_seconds       | 2021-03-12 13:55:09.623 |       2 |      5 |           3 |
 promhttp_metric_handler_req... | 2021-03-12 13:55:09.470 |       2 |      6 |           4 |
 n9e_transfer_stra_err          | 2021-01-27 20:36:36.408 |       2 |      2 |           2 |
 redis_rdb_last_bgsave_durat... | 2021-03-12 13:55:09.612 |       2 |      5 |           3 |
 snmp_tcp_rtomin                | 2021-01-27 20:36:27.450 |       2 |      2 |           3 |
 redis_memory_used_peak_bytes   | 2021-03-12 13:55:09.606 |       2 |      5 |           3 |
 agent_metric_report_cnt        | 2021-01-27 20:36:27.042 |       2 |      2 |           3 |
 net_in_bits_total_percent      | 2021-01-27 20:36:27.678 |       2 |      2 |          21 |
 sys_net_tcp_ip4_con_active     | 2021-01-27 20:36:35.310 |       2 |      2 |          18 |
 redis_aof_current_rewrite_d... | 2021-03-12 13:55:09.720 |       2 |      5 |           3 |
 sys_net_tcp_ip4_con_passive    | 2021-01-27 20:36:35.297 |       2 |      2 |          18 |
 mysql_up                       | 2021-03-12 13:55:09.416 |       2 |      6 |           4 |
 net_out_bits                   | 2021-01-27 20:36:27.586 |       2 |      3 |          21 |
 snmp_tcp_outrsts               | 2021-01-27 20:36:27.438 |       2 |      2 |           3 |
 snmp_udp_incsumerrors          | 2021-01-27 20:36:27.497 |       2 |      2 |           3 |
 n9e_transfer_points_out_judge  | 2021-01-27 22:52:33.397 |       2 |      2 |           1 |
 redis_target_scrape_request... | 2021-03-23 10:18:45.891 |       2 |      5 |           3 |
 net_out_pps                    | 2021-01-27 20:36:27.615 |       2 |      3 |          21 |
 mem_swap_bytes_total           | 2021-01-27 20:36:27.151 |       2 |      2 |           3 |
 net_sockets_used               | 2021-01-27 20:36:27.091 |       2 |      2 |           3 |
 sys_fs_files_free              | 2021-01-27 20:36:27.358 |       2 |      2 |           3 |
 redis_aof_rewrite_in_progress  | 2021-03-12 13:55:09.773 |       2 |      5 |           3 |
 disk_io_avgqu_sz               | 2021-01-27 20:36:27.297 |       2 |      3 |           8 |
 n9e_judge_query_data_by_tra... | 2021-01-27 22:52:33.422 |       2 |      2 |           1 |
 mysql_exporter_last_scrape_... | 2021-03-12 13:55:09.458 |       2 |      6 |           4 |
 redis_memory_used_lua_bytes    | 2021-03-12 13:55:09.679 |       2 |      5 |           3 |
 redis_repl_backlog_history_... | 2021-03-12 13:55:09.899 |       2 |      5 |           3 |
 sys_net_tcp_ip4_con_failures   | 2021-01-27 20:36:35.291 |       2 |      2 |          18 |
 disk_inodes_free               | 2021-01-27 20:36:27.539 |       2 |      3 |           3 |
 n9e_judge_query_data_by_mem    | 2021-01-27 22:52:33.416 |       2 |      2 |           1 |
 n9e_tsdb_get_index_err         | 2021-01-27 20:36:26.842 |       2 |      2 |           1 |
 n9e_index_xclude_qp10s         | 2021-01-27 22:52:33.295 |       2 |      2 |           1 |
 cpu_iowait                     | 2021-01-27 20:36:27.207 |       2 |      2 |           3 |
 disk_cap_bytes_used            | 2021-01-27 20:36:27.562 |       2 |      2 |          21 |
 disk_io_write_msec             | 2021-01-27 20:36:35.233 |       2 |      3 |          32 |
 disk_io_svctm                  | 2021-01-27 20:36:27.308 |       2 |      3 |           8 |
 redis_exporter_build_info      | 2021-03-12 13:55:09.787 |       2 |      9 |           7 |
 redis_uptime_in_seconds        | 2021-03-12 13:55:09.926 |       2 |      5 |           3 |
 n9e_transfer_query_taosd_err   | 2021-02-07 16:24:53.398 |       2 |      2 |           1 |
 snmp_tcp_inerrs                | 2021-01-27 20:36:27.391 |       2 |      2 |           3 |
 redis_aof_enabled              | 2021-03-12 13:55:09.644 |       2 |      5 |           3 |
 sys_ps_process_total           | 2021-01-27 20:36:27.247 |       2 |      2 |           3 |
 mem_swap_bytes_used_percent    | 2021-01-27 20:36:27.167 |       2 |      2 |           3 |
 net_out_errs                   | 2021-01-27 20:36:27.627 |       2 |      3 |          21 |
 redis_rdb_current_bgsave_du... | 2021-03-12 13:55:09.632 |       2 |      5 |           3 |
 n9e_judge_get_index_err        | 2021-01-27 20:36:26.974 |       2 |      2 |           2 |
 n9e_judge_stra_count           | 2021-01-27 20:36:26.968 |       2 |      2 |           2 |
 n9e_judge_running              | 2021-01-27 22:52:33.427 |       2 |      2 |           1 |
 disk_inodes_used_percent       | 2021-01-27 20:36:27.550 |       2 |      3 |           3 |
 mem_swap_bytes_used            | 2021-01-27 20:36:27.156 |       2 |      2 |           3 |
 redis_evicted_keys_total       | 2021-03-23 10:18:45.922 |       2 |      5 |           3 |
 cpu_steal                      | 2021-01-27 20:36:27.228 |       2 |      2 |           3 |
 redis_last_slow_execution_d... | 2021-03-12 13:55:09.905 |       2 |      5 |           3 |
 snmp_tcp_maxconn               | 2021-01-27 20:36:27.401 |       2 |      2 |           3 |
 disk_bytes_total               | 2021-01-27 20:36:27.510 |       2 |      4 |          85 |
 redis_aof_last_rewrite_dura... | 2021-03-12 13:55:09.658 |       2 |      5 |           3 |
 net_sockets_tcp_inuse          | 2021-01-27 20:36:27.099 |       2 |      2 |           3 |
 net_bandwidth_mbits            | 2021-01-27 20:36:27.650 |       2 |      3 |          21 |
 redis_cpu_user_children_sec... | 2021-03-23 10:18:45.955 |       2 |      5 |           3 |
 net_in_bits                    | 2021-01-27 20:36:27.580 |       2 |      3 |          21 |
 n9e_transfer_stra_count        | 2021-01-27 22:52:33.389 |       2 |      2 |           1 |
 redis_loading_dump_file        | 2021-03-12 13:55:09.600 |       2 |      5 |           3 |
 sys_fs_files_used              | 2021-01-27 20:36:27.363 |       2 |      2 |           3 |
 redis_memory_used_rss_bytes    | 2021-03-12 13:55:09.701 |       2 |      5 |           3 |
 n9e_transfer_query_taosd       | 2021-01-27 22:45:43.400 |       2 |      2 |           1 |
 cpu_idle                       | 2021-01-27 20:36:27.178 |       2 |      2 |          21 |
 snmp_udp_indatagrams           | 2021-01-27 20:36:27.461 |       2 |      2 |           3 |
 cpu_loadavg_5                  | 2021-01-27 20:36:27.259 |       2 |      2 |           3 |
 n9e_judge_event_alert          | 2021-01-30 20:37:13.403 |       2 |      2 |           1 |
 snmp_udp_rcvbuferrors          | 2021-01-27 20:36:27.484 |       2 |      2 |           3 |
 n9e_transfer_metric_qp10s      | 2021-01-27 22:45:33.382 |       2 |      2 |           1 |
 n9e_index_persist_err          | 2021-03-26 18:48:38.781 |       2 |      2 |           0 |
 cpu_sys                        | 2021-01-27 20:36:27.199 |       2 |      2 |          21 |
 snmp_tcp_insegs                | 2021-01-27 20:36:27.432 |       2 |      2 |           3 |
 process_max_fds                | 2021-03-12 13:55:09.442 |       2 |      6 |           7 |
 disk_io_read_bytes             | 2021-01-27 20:36:27.280 |       2 |      3 |          40 |
 net_in_dropped                 | 2021-01-27 20:36:27.591 |       2 |      3 |          21 |
 disk_io_read_request           | 2021-01-27 20:36:27.270 |       2 |      3 |          40 |
 net_in_pps                     | 2021-01-27 20:36:27.609 |       2 |      3 |          21 |
 mem_bytes_used_percent         | 2021-01-27 20:36:27.132 |       2 |      2 |          21 |
 redis_pubsub_channels          | 2021-03-12 13:55:09.737 |       2 |      5 |           3 |
 redis_connections_received_... | 2021-03-23 10:18:45.872 |       2 |      5 |           3 |
 sys_fs_files_used_percent      | 2021-01-27 20:36:27.369 |       2 |      2 |           3 |
 disk_io_await                  | 2021-01-27 20:36:27.302 |       2 |      3 |          40 |
 n9e_transfer_data_ui_qp10s     | 2021-01-27 22:45:43.391 |       2 |      2 |           1 |
 n9e_transfer_stra_key          | 2021-01-27 22:52:33.381 |       2 |      2 |           1 |
 proc_agent_alive               | 2021-01-27 20:36:27.173 |       2 |      2 |          21 |
 disk_cap_bytes_used_percent    | 2021-01-27 20:36:27.574 |       2 |      2 |          21 |
 redis_client_biggest_input_buf | 2021-03-12 13:55:09.475 |       2 |      5 |           3 |
 disk_io_write_bytes            | 2021-01-27 20:36:27.286 |       2 |      3 |          40 |
 snmp_udp_outdatagrams          | 2021-01-27 20:36:27.478 |       2 |      2 |           3 |
 cpu_util                       | 2021-01-27 20:36:27.184 |       2 |      2 |          21 |
 mem_bytes_total                | 2021-01-27 20:36:27.110 |       2 |      2 |          21 |
 mem_swap_bytes_free            | 2021-01-27 20:36:27.162 |       2 |      2 |           3 |
 redis_migrate_cached_socket... | 2021-03-12 13:55:09.916 |       2 |      5 |           3 |
 disk_io_avgrq_sz               | 2021-01-27 20:36:27.291 |       2 |      3 |           8 |
 net_sockets_tcp_timewait       | 2021-01-27 20:36:27.105 |       2 |      2 |           3 |
 disk_cap_bytes_free            | 2021-01-27 20:36:27.569 |       2 |      2 |          21 |
 redis_cpu_sys_children_seco... | 2021-03-23 10:18:45.877 |       2 |      5 |           3 |
 sys_net_netfilter_nf_conntr... | 2021-03-12 13:53:39.455 |       2 |      2 |           1 |
 n9e_transfer_judge_queue_err   | 2021-01-27 20:36:36.382 |       2 |      2 |           2 |
 process_virtual_memory_max_... | 2021-03-12 13:55:09.434 |       2 |      6 |           7 |
 redis_slowlog_last_id          | 2021-03-12 13:55:09.589 |       2 |      5 |           3 |
 redis_replica_partial_resyn... | 2021-03-12 13:55:09.760 |       2 |      5 |           3 |
 redis_exporter_last_scrape_... | 2021-03-12 13:55:09.870 |       2 |      6 |           4 |
 net_out_bits_total_percent     | 2021-01-27 20:36:27.684 |       2 |      2 |          21 |
 n9e_index_tag_qp10s            | 2021-01-27 22:46:53.294 |       2 |      2 |           1 |
 redis_repl_backlog_first_by... | 2021-03-12 13:55:09.888 |       2 |      5 |           3 |
Query OK, 235 row(s) in set (0.003023s)
taos> select * from net_in_bits_total >> 20210428.sql;
Query OK, 254069 row(s) in set (1.050476s)

taos> 
</code></pre>
<br>欢迎加群ucGIS 708224555</br>
<img src="https://github.com/xiangxud/ucGIS/blob/main/ucGISEngine/QQID.jpg" width="200"  alt="技术讨论群"/><br/>
[我的博客](https://blog.csdn.net/superxxd)  
