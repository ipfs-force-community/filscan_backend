package offlinereplay

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gorm.io/gorm"
)

// 本文件是「按高度清单跑」（--epochs-file）的执行侧：升序逐个高度各跑一个 Dry 同步器。
//
// 与区间模式的三个差别（都是为「离散缺口补数」服务的）：
//  1. 重试粒度 = 单个高度：区间模式失败会整批重跑（同批已成功的高度会被再写一遍），
//     清单模式一个高度一个同步器，重试只重跑那一个高度；
//  2. 坏高度不阻断整批：同一高度连续失败达上限即放弃（见 epochgate.go 的保险丝），
//     区间模式沿用「失败即整批重试」的原语义，不做放弃；
//  3. 开跑前校验高度不高于聚合器链头：高于链头的高度会让同步器每轮空转等待、永远不退出，
//     那会把「清单写错」变成「整批卡死」，所以一次性拦在开跑前。
//
// 已知代价（写在这里免得被当成故障）：每个高度各起一个同步器，而 syncer.Run() 的首个 tick 是
// 1 秒（timer 初值），因此每个高度有约 1s 的空转等待 —— 1333 个高度约 22 分钟纯等待。
// 换来的是「重试粒度 = 单个高度」「单个坏高度不阻断整批」，对离散补数是划算的（区间模式做不到这两点）。

// listResult 清单模式的逐高度结果
type listResult struct {
	Run       int              // 已跑高度数（含被放弃的）
	Abandoned []AbandonedEpoch // 被放弃的高度（连续失败达上限）
	NotRun    []int64          // 收到退出信号后没跑的高度（升序）
}

// executeList 按清单执行回放（Execute 在 plan.IsList() 时走这里）。
// 返回值：装配/依赖/链头校验失败才返回错误；单个高度失败不返回错误（记进报告，见包注释）。
func executeList(plan Plan, opt RunOptions, target Target, db *gorm.DB, agg londobell.Agg,
	adapter londobell.Adapter, writes func() []WriteStat) error {

	if agg == nil {
		return fmt.Errorf("离线回放需要聚合器客户端（traces/tipset 来源）")
	}
	list := plan.List
	if err := checkHeightsBelowHead(agg, list); err != nil {
		return err
	}

	// 整批共享一个计数器：报告里的聚合器调用次数是清单的累计值（不是单个高度）
	cagg := NewCountingAgg(agg)
	tel := &Telemetry{
		From:    list.Min(),
		To:      list.Max(),
		NoWrite: opt.NoWrite,
		Syncer:  target.Name,
		Tasks:   target.TaskNames(),
		Agg:     cagg,
		List:    &list,
		Writes:  writes,
	}
	gate := newEpochGate(opt.EpochFailLimit)

	log.Printf("模式: 按高度清单跑 —— %s，升序逐个高度各跑一个 Dry 同步器"+
		"（只写派生表；不写同步指针/台账/任务高度）", list)
	if opt.NoWrite {
		log.Printf("模式: --no-write（只统计不落库）—— 派生表写入全部被拦截，仅计数；" +
			"仍会连接数据库读取（只读 SQL），不会产生任何数据变更")
	} else {
		log.Printf("模式: 真写 —— 本次会真实写入派生表（仍不会写同步指针与台账）；若只想测量请加 --no-write")
	}
	log.Printf("坏高度保险丝: 同一高度连续失败 %d 次即放弃该高度并继续后续高度"+
		"（Dry 模式不写跳过台账，被放弃的高度会逐条列进报告）", gate.Limit())

	quit := _app.WaitExitSignal()
	epochs := list.Epochs()
	started := time.Now()
	out := listResult{}
	// 任务与计算器套上保险丝：同一高度连续失败达上限即放弃该高度（不改动调用方传入的 Target）
	gated := gate.wrap(target)

loop:
	for i, h := range epochs {
		// 信号只在两个高度之间生效：当前高度的重试不会被中途打断（避免留下半截写入）
		select {
		case sig := <-quit:
			out.NotRun = append(out.NotRun, epochs[i:]...)
			log.Printf("收到信号 %s，停止后续 %d 个高度（以下统计为部分结果）", sig, len(out.NotRun))
			break loop
		default:
		}

		s, err := assembleEpoch(h, opt, gated, db, cagg, adapter)
		if err != nil {
			return err // 装配失败 = 参数/依赖问题（不是数据问题）⇒ 整批失败
		}
		if err := s.Init(); err != nil {
			return err
		}

		heightStart := time.Now()
		s.Run() // 单高度：该高度跑完（或被放弃）后 epoch 即越过 stopEpoch，Run() 自行返回
		out.Run++

		// 三种结局要分清（否则「重试后成功」会被误读成「没跑成」）：
		// 放弃 / 重试后成功 / 一次成功
		switch f := gate.Failures(h); {
		case gate.abandoned(h):
			log.Printf("[%d/%d] 高度 %d 放弃: 失败 %d 次达上限，该高度仍需重跑，耗时: %s",
				i+1, len(epochs), h, f, time.Since(heightStart))
		case f > 0:
			log.Printf("[%d/%d] 高度 %d 完成（重试 %d 次后成功），耗时: %s",
				i+1, len(epochs), h, f, time.Since(heightStart))
		default:
			log.Printf("[%d/%d] 高度 %d 完成，耗时: %s", i+1, len(epochs), h, time.Since(heightStart))
		}
	}

	out.Abandoned = gate.Abandoned()
	fmt.Println(tel.ReportList(time.Since(started), out))
	return nil
}

// checkHeightsBelowHead 开跑前校验：清单里的高度不得高于聚合器链头。
//
// 为什么必须校验：run() 里 `if s.epoch > finalHeight`（链头）会 sleep 后重试，
// 于是「清单里有一个未来高度」= 那个高度对应的同步器永远不返回 = 整批卡死。
// 这里一次性拦下（报错前不做任何写入），并在错误里点名最高的几个高度。
//
// 注意：预检直接走**未包装**的聚合器 —— 它不属于任何一个高度的回放，不该计入
// 「每高度 1 次 LatestTipset」的调用统计。
func checkHeightsBelowHead(agg londobell.Agg, list EpochList) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tipsets, err := agg.LatestTipset(ctx)
	if err != nil {
		return fmt.Errorf("取聚合器链头失败（清单模式开跑前必须校验高度不高于链头）: %w", err)
	}
	if len(tipsets) != 1 {
		return fmt.Errorf("取聚合器链头失败: 期望 1 个最新 tipset, 实际: %d", len(tipsets))
	}
	head := tipsets[0].ID - 1 // 与 syncer.run() 一致：最终高度 = tipset ID - 1

	var over []int64
	for _, e := range list.Epochs() {
		if e > head {
			over = append(over, e)
		}
	}
	if len(over) == 0 {
		return nil
	}
	return fmt.Errorf("清单里有 %d 个高度高于聚合器链头 %d（首个: %d，最高: %d）: "+
		"高度高于链头时同步器只会空转等待、整批永远跑不完；请从清单中移除这些高度后重跑",
		len(over), head, over[0], over[len(over)-1])
}
