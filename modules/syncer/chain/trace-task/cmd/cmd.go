// Package minergasfeecmd 提供「离线回放指定高度区间 / 高度清单的 Trace 手续费派生数据」子命令。
//
// 用途（主网空洞回补）：把某段高度区间或某份高度清单的「每高度手续费 / 每方法手续费 /
// 每矿工 Gas 消耗」补回来，而**完全不碰**同步指针与台账（chain.sync_syncers /
// chain.sync_task_epochs / chain.sync_syncer_epochs / chain.sync_skipped_epochs 一个都不写）。
//
// 目标表（三张，唯一键情况各不相同 —— 直接决定「能不能重跑」）：
//
//	chain.miner_gas_fees  —— **无唯一键**（migration/1.chain.sql:83-91 没有任何索引）
//	chain.method_gas_fees —— UNIQUE (epoch, method)（migration/1.chain.sql:81）
//	chain.base_gas_costs  —— **无唯一键**，且 acc_messages 是**累计列**（migration/1.chain.sql:27-35）
//
// 对应生产任务：chain 同步器第 1 个任务组的 trace_task
// （modules/syncer/chain/trace-task，Task.Name() = "trace-task"；生产装配见
// injector/syncer_manager.go:169）。它是**同步任务**：对高度连续性没有要求，但**每个高度都写**
// （只受 ctx.Empty() 限制）。
//
// 触发条件：每个高度执行；ctx.Empty() 时直接返回。该高度 traces 为空时任务直接报错
// （trace_task.go:104-106 `traces is emtpy`），由保险丝按「同一高度连续失败 N 次即放弃」处理。
//
// 输入（决定「它依赖谁」）：
//  1. **Datamap 里的 traces**（trace_task.go:98-102；生产由 SetTracesBuilder 注入，
//     即 injector.SetTracesBuilder → agg.Traces(epoch, epoch+1)）—— 外部数据源，不在空洞内。
//     ⇒ 本命令**必须注入 traces**（Target.SkipTraces = false，与生产链同步器一致）。
//  2. 聚合器 agg.AggPreNetFee / agg.AggProNetFee / agg.MinerGasCost（trace_task.go:172/181/190）—— 外部；
//  3. 适配器 adapter.Epoch(epoch) 取该高度的 BaseFee（trace_task.go:144）—— 外部；
//  4. traces 里 SubmitWindowedPoSt 的 GasCost（trace_task.go:244-257）—— 同上，来自 traces；
//  5. **chain.sync_miner_epochs + chain.miner_infos**：MinerGasCostCalculator 要用扇区大小把矿工
//     分成 32G / 64G（miner_gas_cost.go:34 → typer.Typer.MinerSectorSize，typer.go:74-88 →
//     dal_task_change_actor.go:176-196）——取法很特别：先查 `chain.sync_miner_epochs` 里
//     **最大** epoch，再查该 epoch 的 chain.miner_infos；查不到就回退 adapter.Miner(miner).SectorSize。
//     空洞回放时「最大 epoch」通常已是链头（该行本来就在），因此**不构成硬依赖**；
//  6. chain.base_gas_costs 里「epoch < 当前高度」的**上一条**记录（trace_task.go:58
//     GetLastBaseGasCostOrNil，dal_task_syncer_trace.go:44-51），用于累计列 acc_messages。
//
// 输入为空会导致哪些列偏低（务必先读）：
//   - traces 取不到该高度 ⇒ 该高度**整段失败**（不是偏低），按保险丝重试后被放弃并列入报告；
//   - 聚合器三个方法取不到 ⇒ chain.miner_gas_fees 缺行（pre_agg / prove_agg / sector_gas 三项会偏低
//     或缺行），chain.method_gas_fees 不受影响（它只用 traces）；
//   - chain.base_gas_costs 的上一条记录取不到（如只补空洞中段）⇒ chain.base_gas_costs.acc_messages
//     **从零重新累计（偏低）**，messages 不受影响。⇒ 必须按**升序**、从空洞起点连续补；
//   - chain.sync_miner_epochs / chain.miner_infos 的扇区大小取不到且 adapter.Miner 也失败
//     ⇒ 该高度报 `unkonwn sector size` 并失败（不会写错数据，但补不上）；扇区大小取错（不是 32G/64G）
//     同样是该高度失败。sector_gas32 / sector_gas64 / avg_gas_limit32 / avg_gas_limit64 会因此缺失。
//
// 幂等性 / 重跑：
//
//   - chain.method_gas_fees 有唯一键 ⇒ 重跑同一高度会**撞唯一键**（该高度重试后被放弃并列进报告）；
//
//   - chain.miner_gas_fees 与 chain.base_gas_costs **没有唯一键** ⇒ 重跑会**静默追加重复行**：
//     miner_gas_fees 的 Gas 消耗会被多算（calc-miner-owner-task 的 GetMinersAccGasFees 是 sum 聚合，
//     acc_seal_gas / acc_wd_post_gas 会偏高）；base_gas_costs 的 acc_messages 会翻倍。
//     所以重跑前必须按高度删旧行：
//
//     delete from chain.miner_gas_fees where epoch = <高度>;
//     delete from chain.base_gas_costs where epoch = <高度>;
//     delete from chain.method_gas_fees where epoch = <高度>;
//
// 上游依赖（谁会因为本表为空而偏低）：chain.miner_stats / chain.owner_stats
// （calc-miner-owner-task 的 handleMinerAccGasFee 读 chain.miner_gas_fees）⇒ 本命令必须排在它**之前**。
package minergasfeecmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/typer"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/trace-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

type options struct {
	config     string
	start      int64
	end        int64
	epochsFile string
	noWrite    bool
}

// Command 离线回放 Trace 手续费派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "miner-gas-fees [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的 Trace 手续费（补 chain.miner_gas_fees / method_gas_fees / base_gas_costs）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的 Trace 手续费派生数据，补 chain.miner_gas_fees / chain.method_gas_fees /\n" +
			"chain.base_gas_costs（只跑 sync task trace-task，即生产 chain 同步器 1 号任务组的那个任务）。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续空洞用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"与其它离线命令最大的差别：**本任务读 traces** ⇒ 每个高度会真的调一次聚合器 Traces\n" +
			"（并额外调 AggPreNetFee / AggProNetFee / MinerGasCost 与适配器的 Epoch），外部调用量按高度线性增长。\n\n" +
			"输入与偏差（跑之前务必先读）：\n" +
			"  1. chain.base_gas_costs.acc_messages 是累计列，取自「上一条 base_gas_costs」——\n" +
			"     取不到就会从零重新累计（偏低）⇒ 必须按升序、从空洞起点连续补；\n" +
			"  2. 聚合器的 AggPreNetFee / AggProNetFee / MinerGasCost 取不到该高度 ⇒ chain.miner_gas_fees\n" +
			"     缺行或 pre_agg / prove_agg / sector_gas 偏低；\n" +
			"  3. traces 取不到该高度 ⇒ 该高度整段失败（报 `traces is emtpy`），按保险丝重试后放弃并列入报告。\n\n" +
			"幂等性：chain.method_gas_fees(epoch,method) 有唯一键，重跑会撞唯一键；\n" +
			"chain.miner_gas_fees 与 chain.base_gas_costs **没有唯一键**，重跑会静默追加重复行\n" +
			"（Gas 消耗被多算、acc_messages 翻倍）。重跑前先按高度删除这三张表的旧行。\n\n" +
			"--no-write：跑完整条 task 管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
			"预计写入行数），用于安全测量；该模式下不会产生任何数据变更。",
		Run: func(_ *cobra.Command, _ []string) {
			if err := run(option); err != nil {
				log.Fatal(err)
			}
		},
	}

	cmd.Flags().StringVarP(&option.config, "config", "c", "", "配置文件路径")
	cmd.Flags().Int64VarP(&option.start, "start", "s", 0, "起始高度（含）")
	cmd.Flags().Int64VarP(&option.end, "end", "e", 0, "截止高度（含）")
	cmd.Flags().StringVar(&option.epochsFile, "epochs-file", "",
		"高度清单文件（一行一个高度，可含空行与 # 注释；与 --start/--end 互斥；内部升序去重后逐个高度回放）")
	cmd.Flags().BoolVar(&option.noWrite, "no-write", false,
		"只统计不落库：跑完整条 task 管线但不写任何派生表，只输出统计")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

// run 执行一次离线回放。抽成独立函数（而不是全写在 Run 里）是为了让单测能直接断言
// 「参数非法时的报错」，而不必穿过 log.Fatal（那会 os.Exit，测不了）。
func run(option options) error {
	// 计划解析与校验放在最前面：参数不合法就不去连任何生产依赖
	plan, err := resolvePlan(option)
	if err != nil {
		return err
	}

	deps, err := offlinereplay.Connect(option.config)
	if err != nil {
		return err
	}
	defer deps.Close()
	log.Printf("离线回放计划: %s; %s", plan, offlinereplay.ConfLine(deps.Conf))

	target, writes := buildTarget(deps.DB, deps.Adapter, option.noWrite)

	opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
	opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

	return offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
}

// resolvePlan 计划解析的唯一入口（单独抽出来，单测可直接覆盖参数校验）
func resolvePlan(option options) (offlinereplay.Plan, error) {
	return offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
}

// 下面两个包级变量只为让单测注入假仓储 / 假扇区大小来源；生产路径与 NewTraceTask 完全一致。
var (
	newTraceRepo = func(db *gorm.DB) repository.SyncerTraceTaskRepo { return dal.NewSyncerTraceTaskDal(db) }

	// newMinerGasCalc 与生产 trace_task.NewTraceTask 里那一行同构（typer 需要 ChangeActorTask 仓储
	// 来取矿工扇区大小，取不到时 typer 会回退 adapter.Miner）。
	newMinerGasCalc = func(db *gorm.DB, adapter londobell.Adapter) *trace_task.MinerGasCostCalculator {
		return trace_task.NewMinerGasCostCalculator(typer.NewTyper(dal.NewChangeActorTaskDal(db), adapter))
	}
)

// buildTarget 装配回放目标：同步器名与生产一致（chain），只注册 sync task trace-task。
//
// SkipTraces 保持默认 false（= 注入 traces）：trace-task 的第一个动作就是读 Datamap 里的 traces，
// 生产 chain 同步器同样是 WithContextBuilder(SetTracesBuilder)（injector/syncer_manager.go:166）。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, adapter londobell.Adapter, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	target := offlinereplay.Target{
		Name: filscansyncer.ChainSyncer,
	}
	calc := newMinerGasCalc(db, adapter)

	if !noWrite {
		target.Groups = []filscansyncer.TaskGroup{{trace_task.NewTraceTaskWithRepo(newTraceRepo(db), calc)}}
		return target, nil
	}

	wrapped := NewNoWriteTraceRepo(newTraceRepo(db))
	target.Groups = []filscansyncer.TaskGroup{{trace_task.NewTraceTaskWithRepo(wrapped, calc)}}
	return target, wrapped.WriteStats
}
