// Package fnscmd 提供「离线回放指定高度区间的 FNS 派生数据」子命令。
//
// 用途（主网缺口回补）：把某段高度区间的 FNS（域名服务）派生数据补回来，而**完全不碰**同步指针与台账
// （chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs / chain.sync_skipped_epochs 都不写）。
//
// 组件与生产 FnsSyncer 完全一致：
//   - task:       FnsTask            → fns.events
//   - calculator: CalcFnsTask        → fns.tokens / fns.actions / fns.transfers / fns.reverses
//
// 回放顺序注意事项（重要）：计算器是**状态式**的 —— 它按高度顺序读 fns.events 再增删 tokens/actions/
// transfers/reverses（含 DeleteTokenByName、AddFnsReserveDomainWithConflict 这类「先删后插」的覆盖写）。
// 因此：
//   - 回放区间必须从缺口的起点**连续**跑到终点（不要挑着高度跑），否则某个高度的状态可能建立在缺失事件上；
//   - 若这些派生表在缺口区间内已有（部分）数据，回放后可能出现重复的 actions/transfers 行（追加式写入），
//     需要先按区间删除再重放：`delete from fns.actions where epoch between <start> and <end>` 等；
//   - 只想补 fns.events 时可加 --skip-calculator。
package fnscmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-fns-task"
	fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/fns-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

type options struct {
	config         string
	start          int64
	end            int64
	epochsFile     string
	noWrite        bool
	skipCalculator bool
}

// Command 离线回放 FNS 派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "fns [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write] [--skip-calculator]",
		Short: "离线回放指定高度区间/高度清单的 FNS 派生数据（不写同步指针/台账）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的 FNS 派生数据，补\n" +
			"fns.events（task）与 fns.tokens / fns.actions / fns.transfers / fns.reverses（calculator）。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续缺口用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务与计算器（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"计算器是状态式的（按高度顺序读 fns.events 再覆盖写 tokens/reverses、追加写 actions/transfers）：\n" +
			"用区间模式时要从缺口起点连续跑到终点；只需要补 fns.events 时可用 --skip-calculator。\n\n" +
			"--no-write：跑完整条管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
			"预计写入行数），用于安全测量；该模式下不会产生任何数据变更。",
		Run: func(cmd *cobra.Command, args []string) {

			var err error
			defer func() {
				if err != nil {
					log.Fatal(err)
				}
			}()

			// 计划解析与校验放在最前面：参数不合法就不去连任何生产依赖
			plan, err := offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
			if err != nil {
				return
			}

			deps, err := offlinereplay.Connect(option.config)
			if err != nil {
				return
			}
			defer deps.Close()
			log.Printf("离线回放计划: %s; %s", plan, offlinereplay.ConfLine(deps.Conf))

			var abiDecoder fevm.ABIDecoderAPI
			if !option.skipCalculator {
				abiDecoder, err = deps.AbiDecoder()
				if err != nil {
					return
				}
			}

			target, writes := buildTarget(deps.DB, abiDecoder, option.noWrite, option.skipCalculator)

			opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
			opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

			err = offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
		},
	}

	cmd.Flags().StringVarP(&option.config, "config", "c", "", "配置文件路径")
	cmd.Flags().Int64VarP(&option.start, "start", "s", 0, "起始高度（含）")
	cmd.Flags().Int64VarP(&option.end, "end", "e", 0, "截止高度（含）")
	cmd.Flags().StringVar(&option.epochsFile, "epochs-file", "",
		"高度清单文件（一行一个高度，可含空行与 # 注释；与 --start/--end 互斥；内部升序去重后逐个高度回放）")
	cmd.Flags().BoolVar(&option.noWrite, "no-write", false,
		"只统计不落库：跑完整条 task 管线但不写任何派生表，只输出统计")
	cmd.Flags().BoolVar(&option.skipCalculator, "skip-calculator", false,
		"只跑 FnsTask（补 fns.events），不跑 CalcFnsTask（不改 tokens/actions/transfers/reverses）")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

// buildTarget 装配回放目标：同步器名与生产一致（fns），任务 + 计算器与生产 FnsSyncer 一致。
// 前置条件：skipCalculator=false 时 abiDecoder 必须非 nil（计算器用它解码事件）。
// --no-write 时把 FEvmRepo 与 FnsSaver 都换成「只统计不落库」包装（task 与 calculator 共用同一个
// FnsSaver 包装实例，计数因此是二者之和），并返回写统计来源。
func buildTarget(db *gorm.DB, abiDecoder fevm.ABIDecoderAPI, noWrite, skipCalculator bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var fevmRepo repository.FEvmRepo = dal.NewFEvmDal(db)
	var fnsSaver repository.FnsSaver = dal.NewFnsSaverDal(db)

	// --no-write：把两个仓储都换成「只统计不落库」包装；task 与 calculator 共用同一个
	// FnsSaver 包装实例，因此计数是二者写入之和。
	var writes func() []offlinereplay.WriteStat
	if noWrite {
		fe := NewNoWriteFEvmRepo(fevmRepo)
		saver := NewNoWriteFnsSaver(fnsSaver)
		fevmRepo, fnsSaver = fe, saver
		writes = func() []offlinereplay.WriteStat {
			return append(fe.WriteStats(), saver.WriteStats()...)
		}
	}

	target := offlinereplay.Target{
		Name:   filscansyncer.FnsSyncer,
		Groups: []filscansyncer.TaskGroup{{fns_task.NewFnsTask(fevmRepo, fnsSaver)}},
	}
	if skipCalculator {
		// 只补 fns.events：不注册计算器（也就不改 tokens/actions/transfers/reverses）
		return target, writes
	}

	target.Calculators = []filscansyncer.Calculator{calc_fns_task.NewCalcFnsTask(abiDecoder, fnsSaver)}
	return target, writes
}
