// filscan-metrics：「同步落后高度」与「关键表新鲜度」指标采集器。
//
// 背景：2026-09 主网索引链静默停摆 18 天而无有效告警。现役 filscan_syncer_delay_height
// 口径坏（实测 chain=2 / miner=60，真实落后 51393 个高度），四条规则引用的指标在
// Prometheus 里根本不存在。本命令给监控侧一份口径明确、可判、容错的数据源。
//
// 两种用法：
//
//	# 拉模式：暴露 /metrics，给 Prometheus 抓
//	filscan-metrics -c /etc/filscan/config.toml -listen 127.0.0.1:10020
//
//	# 推模式：采集一次打到 stdout，交给现有 pushgateway 推送链路
//	filscan-metrics -c /etc/filscan/config.toml -once \
//	  | curl --data-binary @- http://pushgateway:9091/metrics/job/filscan_metrics
//
// 指标语义（单位一律为「链高度/epoch」）见 modules/metrics/collector.go 的常量注释。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/metrics"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

var (
	// Name 编译期注入的进程名。
	Name string = "filscan-metrics"
	// Version 编译期注入的版本号（Makefile -ldflags）。
	Version string
)

// stringList 支持重复出现的字符串 flag（-table a -table b）。
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	var (
		configFile   string
		listen       string
		once         bool
		ttl          time.Duration
		queryTimeout time.Duration
		tables       stringList
		noLegacy     bool
		showVersion  bool
	)

	fs := flag.NewFlagSet(Name, flag.ExitOnError)
	fs.StringVar(&configFile, "c", "", "配置文件路径（与 filscan-syncer / filscan-api 同一份 config.toml），必填")
	fs.StringVar(&listen, "listen", "", "监听地址；默认取配置 [metrics].address，兜底 127.0.0.1:10020")
	fs.BoolVar(&once, "once", false, "只采集一次并把 Prometheus 文本打到 stdout 后退出（供 pushgateway 推送）")
	fs.DurationVar(&ttl, "interval", 0, "采集/渲染缓存间隔；默认取配置 [metrics].interval 秒，兜底 15s")
	fs.DurationVar(&queryTimeout, "query-timeout", 0, "单条 SQL / 链头接口超时，默认 15s")
	fs.Var(&tables, "table", "关键表新鲜度采集目标，形如 chain.actor_actions:epoch，可重复；默认使用内置清单")
	fs.BoolVar(&noLegacy, "no-legacy", false, "不输出兼容旧规则的 filscan_syncer_delay_height（默认输出，与 filscan_syncer_lag_height 同口径同值）")
	fs.BoolVar(&showVersion, "version", false, "打印版本后退出")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "%s %s\n\n用法：\n", Name, Version)
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatalf("解析参数失败: %s", err)
	}
	if showVersion {
		fmt.Printf("%s %s\n", Name, Version)
		return
	}
	if configFile == "" {
		fs.Usage()
		os.Exit(2)
	}

	conf := new(config.Config)
	if err := _config.UnmarshalConfigFile(configFile, conf); err != nil {
		log.Fatalf("读取配置文件失败: %s", err)
	}
	if conf.DSN == nil || *conf.DSN == "" {
		log.Fatalf("配置缺少数据库 DSN")
	}

	network := metrics.NetworkName(conf.TestNet)

	// 关键表清单：命令行 > 配置 [metrics].tables > 内置默认
	var specs []metrics.TableSpec
	switch {
	case len(tables) > 0:
		var err error
		specs, err = metrics.ParseTableSpecs([]string(tables))
		if err != nil {
			log.Fatalf("非法的 -table 参数: %s", err)
		}
	case len(conf.MetricsTables()) > 0:
		var err error
		specs, err = metrics.ParseTableSpecs(conf.MetricsTables())
		if err != nil {
			log.Fatalf("非法的 [metrics].tables 配置: %s", err)
		}
	}

	// 其它可覆写项：默认取配置，再兜底常量
	if listen == "" {
		listen = conf.MetricsAddress(DefaultListenAddress)
	}
	if ttl <= 0 {
		ttl = time.Duration(conf.MetricsIntervalSeconds()) * time.Second
	}
	if ttl <= 0 {
		ttl = metrics.DefaultTTL
	}
	if queryTimeout <= 0 {
		queryTimeout = time.Duration(conf.MetricsQueryTimeoutSeconds()) * time.Second
	}
	if queryTimeout <= 0 {
		queryTimeout = metrics.DefaultQueryTimeout
	}
	legacy := !noLegacy
	if v := conf.MetricsLegacyDelayName(); v != nil {
		legacy = *v && !noLegacy
	}

	started := time.Now()
	logger := log.New(os.Stdout, fmt.Sprintf("[%s] ", Name), log.LstdFlags)
	logger.Printf("%s %s 启动，网络=%s，数据库=%s", Name, Version, network, maskDSN(*conf.DSN))

	db, cleanup, err := injector.NewGormDB(conf)
	if err != nil {
		log.Fatalf("连接数据库失败: %s", err)
	}
	defer cleanup()

	adapter, err := injector.NewLondobellAdapter(conf)
	if err != nil {
		// 链头不可用不该让进程起不来：采集器会以 up=0 如实暴露，
		// 表新鲜度部分仍然可用（这是「哪张表停在多少高度」那一层）。
		logger.Printf("初始化 londobell adapter 失败，链头高度将不可用: %s", err)
		adapter = nil
	}

	if conf.TestNet {
		chain.RegisterNet(conf.TestNet)
		if adapter != nil {
			// 与 cmd/monitor 一致：测试网用一次真实高度校准基准时间。
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if reply, e := adapter.Epoch(ctx, nil); e == nil && reply != nil {
				chain.RegisterBaseTime(reply.Epoch, reply.BlockTime)
			} else if e != nil {
				logger.Printf("校准测试网基准时间失败: %s", e)
			}
			cancel()
		}
	}

	collector := metrics.NewCollector(
		metrics.NewDBQuerier(db, adapter, queryTimeout),
		network,
		specs,
		metrics.WithLegacyDelayName(legacy),
	)
	service := metrics.NewService(collector, ttl, logger)

	logger.Printf("采集器就绪：关键表 %d 张、间隔 %s、单查询超时 %s、旧指标名兼容=%v（启动耗时 %s）",
		len(collector.Tables()), ttl, queryTimeout, legacy, time.Since(started))

	if once {
		body := service.Render(context.Background(), true)
		if _, err := os.Stdout.Write(body); err != nil {
			log.Fatalf("写 stdout 失败: %s", err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := service.Listen(ctx, listen); err != nil {
		log.Fatalf("服务退出: %s", err)
	}
}

// DefaultListenAddress 监听地址兜底值。只绑本地：链头/表高度数据虽不敏感，
// 但没有任何理由把它暴露到公网（需要跨机抓取时显式 -listen 0.0.0.0:PORT）。
const DefaultListenAddress = "127.0.0.1:10020"

// maskDSN 只打印 DSN 的库名/Host 部分，避免把口令写进日志。
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	if i := strings.Index(dsn, "@"); i >= 0 {
		head := dsn[:i]
		if j := strings.LastIndex(head, ":"); j >= 0 {
			dsn = head[:j] + ":***" + dsn[i:]
		} else {
			dsn = "***" + dsn[i:]
		}
	}
	return dsn
}
