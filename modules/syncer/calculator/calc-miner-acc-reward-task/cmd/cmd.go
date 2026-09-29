// Package mineraccrewardcmd 提供「离线回放指定高度区间 / 高度清单的矿工奖励统计派生数据」子命令。
//
// 用途（主网空洞回补）：把某段高度区间或某份高度清单的 chain.miner_reward_stats 补回来，
// 而**完全不碰**同步指针与台账（chain.sync_syncers / chain.sync_task_epochs /
// chain.sync_syncer_epochs / chain.sync_skipped_epochs 一个都不写）。
//
// 目标表：chain.miner_reward_stats（**没有唯一键**，migration/1.chain.sql:146-151）
// —— 每个高度写 4 行，interval 分别是 24h / 7d / 30d / 1y。
//
// 对应生产计算器：chain 同步器的 calc-miner-acc-reward-task
// （modules/syncer/calculator/calc-miner-acc-reward-task，Calculator.Name() = "calc-miner-acc-reward-task"；
// 生产装配见 injector/syncer_manager.go:181）。它是**计算器**（对高度连续性有依赖），
// 但与其它计算器不同：**每个高度都写**（只受 ctx.Empty() 限制），四个 interval 各一行。
//
// 触发条件：每个高度执行；ctx.Empty() 时直接返回，不写任何表。
//
// 输入（决定「它依赖谁」）：
//  1. **chain.miner_rewards**：四个 interval 各做一次区间求和
//     （prepareAccReward → repo.SumRewards，dal_task_reward.go:53-67
//     `select greatest(sum(reward),0) from chain.miner_rewards where epoch >= ? and epoch <= ?`），
//     区间分别是 [epoch-2880, epoch] / [epoch-2880*7, epoch] / [epoch-2880*30, epoch] / [epoch-2880*365, epoch]
//     ⇒ **本表在空洞里为空**，必须先用 miner-rewards 子命令补上 chain.miner_rewards；
//  2. **chain.builtin_actor_states**：repo.GetNetQualityAdjPower(epoch) 取该高度的全网有效算力
//     （dal_task_reward.go:36-51，`state->>'TotalQualityAdjPower'`，actor = 存储算力 actor）
//     ⇒ 该表由 chain 同步器的 baseline-task 写，**同样在空洞里为空**，必须先用 baseline-actors 子命令补上。
//
// 输入为空会导致哪些列偏低（务必先读）：
//   - chain.miner_rewards 为空（没先回放）⇒ AccReward = 0：**四个 interval 全为 0**，是典型的
//     「看着有数、数值为 0」——所以本命令必须排在 miner-rewards **之后**；
//   - chain.builtin_actor_states 为空（没先回放）⇒ power = 0 ⇒ AccRewardPerT 保持 0（偏低），
//     AccReward 仍然正确 —— 所以本命令必须排在 baseline-actors **之后**；
//   - 这两个输入**都不是空洞里的唯一依赖**：chain.miner_rewards 又依赖 reward-task；
//     chain.builtin_actor_states 又依赖 baseline-task（两者的输入都只有聚合器/适配器，不在空洞内）。
//
// 是否读 traces：**不读**（既不读 Datamap，也不用聚合器与适配器）⇒ Target.SkipTraces = true，
// 省掉每个高度一次 Traces 调用。
//
// 幂等性 / 重跑：chain.miner_reward_stats **没有唯一键**，重跑同一高度会**静默追加重复行**
// （同一 (epoch, interval) 出现多行，按 interval 取数的下游会翻倍/取错行）。重跑前必须按高度删：
//
//	delete from chain.miner_reward_stats where epoch = <高度>;
//
// 上游依赖（谁会因为本表为空而偏低）：Pro / 首页「单T收益」相关查询按 (epoch, interval) 取本表
// （无同步链上的下游任务）。
package mineraccrewardcmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_miner_acc_reward_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-acc-reward-task"
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

// Command 离线回放矿工奖励统计派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "miner-acc-reward [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的矿工奖励统计（补 chain.miner_reward_stats）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的矿工奖励统计，补 chain.miner_reward_stats（每高度 4 行：24h/7d/30d/1y）。\n" +
			"只跑计算器 calc-miner-acc-reward-task（生产 chain 同步器注册的那个计算器）。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续空洞用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行计算器（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"前置（顺序不能错）：\n" +
			"  1. 先跑 miner-rewards 补齐 chain.miner_rewards —— 否则本命令算出来的 AccReward 全是 0（偏低）；\n" +
			"  2. 先跑 baseline-actors 补齐 chain.builtin_actor_states —— 否则 AccRewardPerT 为 0（偏低）。\n\n" +
			"本计算器不读 traces、不用聚合器与适配器 ⇒ 不注入 traces（省掉每高度一次 Traces 调用）。\n\n" +
			"幂等性：chain.miner_reward_stats **没有唯一键**，重跑同一高度会静默追加重复行\n" +
			"（同一 (epoch, interval) 出现多行）。重跑前先按高度删除旧行：\n" +
			"delete from chain.miner_reward_stats where epoch = <高度>;\n\n" +
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

// newRewardRepo 构造爆块奖励仓储。抽成包级变量只是为了让单测能注入假仓储
// （计算器要读 chain.miner_rewards 与 chain.builtin_actor_states，离线的假连接无法应答那些 SQL），
// 生产路径始终是 dal.NewRewardTaskDal。
var newRewardRepo = func(db *gorm.DB) repository.RewardTask { return dal.NewRewardTaskDal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（chain），只注册计算器 calc-miner-acc-reward-task。
// SkipTraces = true：该计算器既不读 Datamap 里的 traces，也不用聚合器与适配器。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.RewardTask = newRewardRepo(db)

	target := offlinereplay.Target{
		Name:       filscansyncer.ChainSyncer,
		SkipTraces: true,
	}

	if !noWrite {
		target.Calculators = []filscansyncer.Calculator{calc_miner_acc_reward_task.NewCalcMinerAccRewardTask(repo)}
		return target, nil
	}

	wrapped := NewNoWriteRewardRepo(repo)
	target.Calculators = []filscansyncer.Calculator{calc_miner_acc_reward_task.NewCalcMinerAccRewardTask(wrapped)}
	return target, wrapped.WriteStats
}
