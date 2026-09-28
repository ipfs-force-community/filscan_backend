// Package evmtransfercmd 提供「离线回放指定高度区间的 EVM 转账派生数据」子命令。
//
// 用途（主网缺口回补）：把某段高度区间的 fevm.evm_transfers / fevm.evm_transfer_stats
// 派生数据补回来，而**完全不碰**同步指针与台账 —— 即 chain.sync_syncers、
// chain.sync_task_epochs、chain.sync_syncer_epochs、chain.sync_skipped_epochs 一个都不写。
//
// 为什么必须走本命令，而不能靠「把 chain.sync_syncers.epoch 回拨到缺口起点重跑」：
// 所有 fevm 派生同步器都共用 injector.SetTracesBuilder，而非 Dry 模式下它会调用
// ctx.Adapter().Epoch(&epoch) 去校验该高度的 tipset；对历史缺口高度该调用必然失败
// （load state tree: failed to load hamt node —— 本地节点只保留近期状态窗口，历史状态已被裁掉），
// 于是回拨指针只会让同步器卡在缺口高度上反复重试，永远追不上。
//
// 两种模式：
//   - 默认（真写）：跑完区间并把派生数据写进 fevm.evm_transfers / fevm.evm_transfer_stats；
//   - --no-write：跑完整条 task 管线（含聚合器取数、actor 查询、聚合统计），
//     但派生表的写入与删除全部被拦下，只统计「调用次数 / 行数 / 聚合器调用次数 / 耗时」。
//     用于上线前安全测量压力与预计写入量，不产生任何数据变更。
package evmtransfercmd

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

type options struct {
	config  string
	start   int64
	end     int64
	noWrite bool
}

// Command 离线回放 EVM 转账派生数据（指定高度区间，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use:   "evm-transfer [-c|--config /path/to/config.toml] --start <高度> --end <高度> [--no-write]",
		Short: "离线回放指定高度区间的 EVM 转账派生数据（不写同步指针/台账）",
		Long: "离线回放 [--start, --end] 高度区间（左闭右闭）的 fevm EVM 转账派生数据，\n" +
			"补 fevm.evm_transfers / fevm.evm_transfer_stats。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"--no-write：跑完整条管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
			"预计写入行数），用于安全测量；该模式下不会产生任何数据变更。",
		Run: func(cmd *cobra.Command, args []string) {

			var err error
			defer func() {
				if err != nil {
					log.Fatal(err)
				}
			}()

			// 区间解析与校验放在最前面：参数不合法就不去连任何生产依赖
			rng, err := ResolveRange(option.start, option.end)
			if err != nil {
				return
			}

			conf := &config.Config{}
			err = _config.UnmarshalConfigFile(option.config, conf)
			if err != nil {
				return
			}
			log.Printf("离线回放高度区间: %s; %s", rng, confLine(conf))

			db, cancel, err := injector.NewGormDB(conf)
			if err != nil {
				return
			}
			defer func() {
				cancel()
			}()

			agg, err := injector.NewLondobellAgg(conf)
			if err != nil {
				return
			}

			adapter, err := injector.NewLondobellAdapter(conf)
			if err != nil {
				return
			}

			var chunk, threshold int64
			if conf.Syncer != nil {
				chunk = derefInt64(conf.Syncer.EpochsChunk)
				threshold = derefInt64(conf.Syncer.EpochsThreshold)
			}

			s, tel, err := BuildSyncer(RunOptions{
				From:            rng.From.Int64(),
				To:              rng.To.Int64(),
				NoWrite:         option.noWrite,
				EpochsChunk:     chunk,
				EpochsThreshold: threshold,
			}, db, agg, adapter, nil)
			if err != nil {
				return
			}

			err = s.Init()
			if err != nil {
				return
			}

			if option.noWrite {
				log.Printf("模式: --no-write（只统计不落库）—— 派生表写入全部被拦截，仅计数；" +
					"仍会连接数据库读取（只读 SQL），不会产生任何数据变更")
			} else {
				log.Printf("模式: 真写 —— 本次会真实写入 fevm.evm_transfers / fevm.evm_transfer_stats" +
					"（仍不会写同步指针与台账）；若只想测量请加 --no-write")
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
		},
	}

	cmd.Flags().StringVarP(&option.config, "config", "c", "", "配置文件路径")
	cmd.Flags().Int64VarP(&option.start, "start", "s", 0, "起始高度（含）")
	cmd.Flags().Int64VarP(&option.end, "end", "e", 0, "截止高度（含）")
	cmd.Flags().BoolVar(&option.noWrite, "no-write", false,
		"只统计不落库：跑完整条 task 管线但不写任何派生表，只输出统计")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")

	return cmd
}

// derefInt64 安全解引用可选的 int64 配置项（缺失/未配置时返回 0，由 BuildSyncer 回退默认值）
func derefInt64(p *int64) (v int64) {
	if p == nil {
		return 0
	}
	return *p
}

// confLine 打印「连的是谁」，便于运维在跑之前核对指向（不打印 DSN，避免泄露口令）
func confLine(conf *config.Config) string {
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
