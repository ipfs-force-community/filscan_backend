// Package minergasestimatecmd 提供「离线回放指定高度区间 / 高度清单的矿工扇区费估算」子命令。
//
// 用途（主网空洞回补）：把某段高度区间或某份高度清单的 chain.base_gas_costs.sector_fee32 /
// sector_fee64 补算回来，而**完全不碰**同步指针与台账（chain.sync_syncers /
// chain.sync_task_epochs / chain.sync_syncer_epochs / chain.sync_skipped_epochs 一个都不写）。
//
// 目标表：chain.base_gas_costs（**没有唯一键**；sector_fee32 / sector_fee64 两列由
// migration/12.miner_sector_gas.sql 追加）—— 本命令只 **UPDATE 当前高度那一行**，
// 不插入新行（行由 trace-task 插入）。
//
// 对应生产计算器：chain 同步器的 calc-estimate-miner-gas
// （modules/syncer/calculator/calc-estimate-miner-gas，Calculator.Name() = "calc-estimate-miner-gas"；
// 生产装配见 injector/syncer_manager.go:182）。它是**计算器**，**每个高度都执行**。
//
// 触发条件：每个高度执行；ctx.Empty() 时直接返回。
//
// 输入（决定「它依赖谁」）：
//  1. **chain.base_gas_costs**：取「最近 960 个高度」的窗口
//     （calc-estimate-miner-gas.go:40 `GetBaseGasCosts(ctx, epoch-120*8, epoch)`；
//     SQL 是 dal_task_syncer_trace.go:136 `epoch > 起 and epoch <= 止 order by epoch desc`
//     ⇒ **包含当前高度那一行**，且 items[0] 就是最新一行），用其中的
//     base_gas / avg_gas_limit32 / avg_gas_limit64 算扇区费。
//     ⇒ 这些行由 trace-task 写（本仓 miner-gas-fees 子命令负责补）——**本命令必须排在它之后**。
//
// 输入为空会导致哪些列偏低（务必先读）：
//   - 窗口里一行都没有（最近的 chain.base_gas_costs 还没补）⇒ 计算器**直接返回、什么都不写**
//     （calc-estimate-miner-gas.go:44-47）⇒ sector_fee32 / sector_fee64 保持 0/NULL：这是
//     「看着跑完了、其实什么都没算」的典型，所以必须先跑 miner-gas-fees；
//   - 窗口里缺了中间若干高度（只补了一部分）⇒ 平均 GasLimit 被少量样本决定 ⇒ sector_fee32/64
//     偏离真值（偏高或偏低，取决于缺的是哪些高度）；窗口右端缺「当前高度那一行」时
//     baseFee 会取到窗口里的最新一行（近似值），偏差通常很小。
//
// 是否读 traces：**不读**（不用聚合器与适配器）⇒ Target.SkipTraces = true。
//
// 幂等性 / 重跑：本命令只做 `update chain.base_gas_costs set sector_fee32=?, sector_fee64=? where epoch=?`
// （dal_task_syncer_trace.go:148）⇒ **不插入、不撞唯一键、可安全重跑**（同一输入算出同一结果）。
// 唯一要注意的：如果该 epoch 因为 trace-task 重跑而出现了**多行**（该表没有唯一键），
// 这条 UPDATE 会把同 epoch 的所有行都改写 —— 重跑 trace-task 前请先按高度删旧行。
package minergasestimatecmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_estimate_miner_gas "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-estimate-miner-gas"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

type options struct {
	config     string
	start      int64
	end        int64
	epochsFile string
	noWrite    bool
}

// Command 离线回放矿工扇区费估算（指定高度区间或高度清单，只改派生表的两列）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "miner-gas-estimate [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的矿工扇区费估算（补 chain.base_gas_costs.sector_fee32/64）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的矿工扇区费估算，更新 chain.base_gas_costs 的 sector_fee32 / sector_fee64\n" +
			"（只跑计算器 calc-estimate-miner-gas，即生产 chain 同步器注册的那个计算器）。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续空洞用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行计算器（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"前置：先跑 miner-gas-fees 补齐 chain.base_gas_costs（本计算器取「最近 960 个高度」的窗口，\n" +
			"窗口里一行都没有时它会**直接返回、什么都不写**）。\n\n" +
			"幂等性：本命令只 UPDATE 当前高度那一行，不插入新行 ⇒ 可安全重跑；\n" +
			"但若该高度因 trace-task 重跑而出现多行（该表无唯一键），UPDATE 会改写同高度的所有行。\n\n" +
			"--no-write：跑完整条计算器管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
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
		"只统计不落库：跑完整条计算器管线但不写任何派生表，只输出统计")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

// run 执行一次离线回放（参数非法时不连任何生产依赖）
func run(option options) error {
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

	target, writes := buildTarget(deps.DB, option.noWrite)

	opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
	opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

	return offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
}

// resolvePlan 计划解析的唯一入口（单独抽出来，单测可直接覆盖参数校验）
func resolvePlan(option options) (offlinereplay.Plan, error) {
	return offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
}

// newTraceRepo 构造 trace 任务仓储（本计算器只用它的 GetBaseGasCosts / UpdateBaseGasCostSectorGas）。
// 抽成包级变量只是为了让单测能注入假仓储（离线的假连接无法应答那些 SQL）。
var newTraceRepo = func(db *gorm.DB) repository.SyncerTraceTaskRepo { return dal.NewSyncerTraceTaskDal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（chain），只注册计算器 calc-estimate-miner-gas。
// SkipTraces = true：该计算器不读 traces，也不用聚合器与适配器。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.SyncerTraceTaskRepo = newTraceRepo(db)

	target := offlinereplay.Target{
		Name:       filscansyncer.ChainSyncer,
		SkipTraces: true,
	}

	if !noWrite {
		target.Calculators = []filscansyncer.Calculator{calc_estimate_miner_gas.NewCalEstimateMinerGas(repo)}
		return target, nil
	}

	wrapped := NewNoWriteTraceRepo(repo)
	target.Calculators = []filscansyncer.Calculator{calc_estimate_miner_gas.NewCalEstimateMinerGas(wrapped)}
	return target, wrapped.WriteStats
}
