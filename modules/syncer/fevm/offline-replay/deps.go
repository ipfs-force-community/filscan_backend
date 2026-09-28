package offlinereplay

import (
	"fmt"
	"log"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"gorm.io/gorm"
)

// 本文件是「离线回放」工具链的公共装配与执行骨架：三个子命令只声明自己的 Target
// （同步器名 + 任务/计算器 + 「只统计不落库」包装），依赖与运行流程完全一致。

// Deps 离线回放需要的外部依赖（与生产同步器同一套构造器，指向同一份配置）
type Deps struct {
	Conf    *config.Config
	DB      *gorm.DB
	Agg     londobell.Agg
	Adapter londobell.Adapter

	cancel func()
}

// Connect 读配置并连上数据库 / 聚合器 / 适配器。
// 任何一步失败都立刻关闭已开资源并返回错误（离线工具不做静默兜底）。
func Connect(confPath string) (*Deps, error) {
	conf := &config.Config{}
	if err := _config.UnmarshalConfigFile(confPath, conf); err != nil {
		return nil, fmt.Errorf("读取配置 %s 失败: %w", confPath, err)
	}

	db, cancel, err := injector.NewGormDB(conf)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	agg, err := injector.NewLondobellAgg(conf)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("连接聚合器失败: %w", err)
	}

	adapter, err := injector.NewLondobellAdapter(conf)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("连接适配器失败: %w", err)
	}

	return &Deps{Conf: conf, DB: db, Agg: agg, Adapter: adapter, cancel: cancel}, nil
}

// Close 释放数据库连接池
func (d *Deps) Close() {
	if d == nil || d.cancel == nil {
		return
	}
	d.cancel()
}

// EpochsConfig 取配置里的并发高度数 / 单批区间上限（缺失时返回 0，由 Assemble 回退默认值）
func (d *Deps) EpochsConfig() (chunk, threshold int64) {
	if d == nil || d.Conf == nil || d.Conf.Syncer == nil {
		return 0, 0
	}
	return DerefInt64(d.Conf.Syncer.EpochsChunk), DerefInt64(d.Conf.Syncer.EpochsThreshold)
}

// AbiDecoder ABI 解码器（erc20 / fns 链路需要：解码事件与合约方法签名）
func (d *Deps) AbiDecoder() (fevm.ABIDecoderAPI, error) {
	if d == nil || d.Conf == nil || d.Conf.ABIDecoderRPC == nil {
		return nil, fmt.Errorf("配置缺少 abi_decoder_rpc，无法构造 ABI 解码器")
	}
	return injector.NewAbiDecoderClient(d.Conf)
}

// AbiNode ABI 依赖的 lotus 节点（erc20 链路需要：读代币名称/精度/余额）
func (d *Deps) AbiNode() (*lotus_api.Node, error) {
	if d == nil || d.Conf == nil || d.Conf.ABINode == nil {
		return nil, fmt.Errorf("配置缺少 abi_node，无法构造节点客户端")
	}
	token := ""
	if d.Conf.ABINodeToken != nil {
		token = *d.Conf.ABINodeToken
	}
	return lotus_api.NewBasicAuthLotusApi("offline-replay", *d.Conf.ABINode, lotus_api.WithAuth("", token))
}

// Execute 跑完整个高度区间并打印统计报告（阻塞直到区间跑完或收到退出信号）。
func Execute(s *syncer.Syncer, tel *Telemetry, opt RunOptions) error {
	rng, err := ResolveRange(opt.From, opt.To)
	if err != nil {
		return err
	}
	if err := s.Init(); err != nil {
		return err
	}

	if opt.NoWrite {
		log.Printf("模式: --no-write（只统计不落库）—— 派生表写入全部被拦截，仅计数；" +
			"仍会连接数据库读取（只读 SQL），不会产生任何数据变更")
	} else {
		log.Printf("模式: 真写 —— 本次会真实写入派生表（仍不会写同步指针与台账）；若只想测量请加 --no-write")
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run()
	}()

	select {
	case <-done:
		log.Printf("离线回放结束: %s", rng)
	case sig := <-_app.WaitExitSignal():
		log.Printf("收到信号 %s，停止等待（以下统计为部分结果）", sig)
	}

	fmt.Println(tel.Report(rng, time.Since(started)))
	return nil
}

// DerefInt64 安全解引用可选 int64 配置项（缺失时返回 0，交由 Assemble 回退默认值）
func DerefInt64(p *int64) (v int64) {
	if p == nil {
		return 0
	}
	return *p
}

// ConfLine 打印「连的是谁」，便于跑之前核对指向（不打印 DSN，避免泄露口令）
func ConfLine(conf *config.Config) string {
	deref := func(p *string) string {
		if p == nil || *p == "" {
			return "<未配置>"
		}
		return *p
	}
	if conf == nil || conf.Londobell == nil {
		return "聚合器=<未配置> 适配器=<未配置>"
	}
	return fmt.Sprintf("聚合器=%s 适配器=%s", deref(conf.Londobell.AggAddress), deref(conf.Londobell.AdapterAddress))
}
