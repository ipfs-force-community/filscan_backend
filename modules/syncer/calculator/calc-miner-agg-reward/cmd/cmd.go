// Package mineraggrewardcmd 提供「离线回放指定高度区间的矿工历史聚合奖励」子命令。
//
// 用途（主网空洞回补）：把某段高度区间涉及过的矿工的 chain.miner_agg_rewards 重新聚合一遍，
// 而**完全不碰**同步指针与台账（chain.sync_syncers / chain.sync_task_epochs /
// chain.sync_syncer_epochs / chain.sync_skipped_epochs 一个都不写）。
//
// 目标表：chain.miner_agg_rewards（**主键 = miner**，migration/23.pro_miner.sql:1-12）
// —— 注意这不是按高度分区的表，而是「每个矿工一条」的**全表聚合**：
//
//	miner / agg_reward / agg_block / agg_win_count
//
// 对应生产计算器：chain 同步器的 calc-miner-agg-reward
// （modules/syncer/calculator/calc-miner-agg-reward，Calculator.Name() = "calc-miner-agg-reward"；
// 生产装配见 injector/syncer_manager.go:183）。它是**计算器**，且**只在 ctx.LastCalc() 时执行**
// （calc-miner-agg-reward.go:35-37）——即「本批高度里最后那个高度」。
//
// 触发条件：仅在批内最后一个高度执行（清单模式每高度一个同步器 ⇒ 每个高度都是批内最后一个）。
//
// 输入（决定「它依赖谁」）：
//  1. **chain.miner_rewards**：先是 GetRewardMiners(ctx.Epochs()) 取「本批高度区间内出现过的矿工」
//     （dal_task_reward.go:194-203，SQL 是 `epoch >= 起 and epoch < 止`）；
//  2. **chain.miner_rewards**：再对这些矿工做**全表**汇总
//     （SumMinersTotalRewards，dal_task_reward.go:207-245，`where miner in (...) group by miner`）；
//  3. **chain.miner_win_counts**：同一批矿工的赢票全表汇总（同一个方法里的第二条 SQL）。
//
// 数组为空会导致哪些列偏低：chain.miner_rewards / chain.miner_win_counts 为空（没先回放）⇒
// 聚合出来的 agg_reward / agg_block / agg_win_count 会**比真实值小**（甚至整批矿工没有行），
// 于是原本正确的历史值会被**改小** —— 所以本命令必须排在 miner-rewards **之后**，
// 且**不要在 miner_rewards 缺数时执行**（它写的是覆盖式的 upsert，会把旧值改坏）。
//
// 是否读 traces：**不读**（也没有聚合器/适配器调用）⇒ Target.SkipTraces = true。
//
// 幂等性 / 重跑：SaveMinerAggReward 是 `insert ... on conflict (miner) do update`
// （dal_task_reward.go:248-263）⇒ 重跑**不会**撞主键、**不会**追加重复行，只会用「当前全表」
// 重新算一遍并覆盖。因此本表**不需要先删区间**。代价是「重跑一遍 = 把历史值改写为当前库的状态」。
//
// **为什么不支持 --epochs-file（高度清单）**：见 validatePlan —— 该计算器按 ctx.Epochs() 取矿工，
// 而清单模式每个高度各起一个同步器（区间 = [h, h]），`epoch >= h and epoch < h` 恒为空
// ⇒ 跑完零写入，是典型的「看着跑成功了、其实什么都没补」。本命令宁可报错也不静默空跑。
//
// 已知偏差（既有实现，忠实复现、不修改）：GetRewardMiners 的 SQL 把右端当**开区间**
// （dal_task_reward.go:200 `epoch < ?`），而调用方传的是左闭右闭的批区间
// ⇒ **每批最后一个高度上爆块的矿工不会被纳入本批聚合**。想彻底避免这一点，
// 请用 1 个高度的批（把 --epochs-chunk 调到 1）或跑一个覆盖更大的区间后再核对。
//
// 另记（不影响写入，但值得修）：chain.miner_agg_rewards 的 po.TableName() 写成了
// "chain.miner_age_rewards"（modules/common/infra/po/chain_miner_agg_reward.go:12，拼写错误），
// 好在写入走的是 dal 里的手写 SQL（真实表名 chain.miner_agg_rewards），所以目前只影响
// 任何「用 gorm + 该 po」去读写它的代码路径。
package mineraggrewardcmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_miner_agg_reward "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-agg-reward"
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

// Command 离线回放矿工历史聚合奖励（区间模式；清单模式被显式拒绝，原因见包注释）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use:   "miner-agg-reward [-c|--config /path/to/config.toml] --start <高度> --end <高度> [--no-write]",
		Short: "离线回放指定高度区间的矿工历史聚合奖励（补 chain.miner_agg_rewards；只支持区间模式）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）的矿工历史聚合奖励，补 chain.miner_agg_rewards\n" +
			"（只跑计算器 calc-miner-agg-reward，即生产 chain 同步器注册的那个计算器）。\n\n" +
			"**本命令只支持区间模式**：该计算器按 ctx.Epochs() 取「本批出现过的矿工」，\n" +
			"而清单模式（--epochs-file）每个高度各起一个同步器（区间 = [h, h]）⇒ 取矿工恒为空 ⇒ 零写入。\n" +
			"给出 --epochs-file 会直接报错，不静默空跑。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行计算器（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"前置（顺序不能错）：必须先跑 miner-rewards 补齐 chain.miner_rewards / chain.miner_win_counts，\n" +
			"否则本命令会把这些矿工的历史聚合值**改小**（本表是覆盖式 upsert）。\n\n" +
			"幂等性：chain.miner_agg_rewards 主键是 miner，写入是 on conflict do update ⇒ 重跑不撞键、\n" +
			"不追加重复行，无需先删区间；代价是「重跑一遍 = 用当前全表重算并覆盖」。\n\n" +
			"已知偏差：该计算器的取矿工 SQL 把右端当开区间（epoch < 止），而调用方传的是左闭右闭的批区间\n" +
			"⇒ 每批最后一个高度上爆块的矿工不会进入本批聚合。必要时跑更大的区间或减小批大小来规避。\n\n" +
			"--no-write：跑完整条计算器管线但不写任何派生表，只输出统计，用于安全测量。",
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
		"高度清单文件（本命令**不支持**：脚本化调用请改用 --start/--end；给出即报错，原因见 --help）")
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
	if err := validatePlan(plan); err != nil {
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

// resolvePlan 计划解析的唯一入口（区间/清单的互斥由框架统一判定）
func resolvePlan(option options) (offlinereplay.Plan, error) {
	return offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
}

// validatePlan 拒绝清单模式。宁可报错也不静默空跑：
// 该计算器取矿工用 `epoch >= 批起 and epoch < 批止`，清单模式每高度一个同步器（批区间 = [h, h]）
// ⇒ 恒为空 ⇒ GetRewardMiners 返回空 ⇒ SumMinersTotalRewards 无行 ⇒ SaveMinerAggReward 零写入。
// 报告里会显示「已跑 N/N 个高度、放弃 0」，很容易被误读成「补上了」。
func validatePlan(plan offlinereplay.Plan) error {
	if plan.IsList() {
		return fmt.Errorf("miner-agg-reward 不支持高度清单模式（--epochs-file）: " +
			"该计算器按批区间 ctx.Epochs() 取「本批出现过的矿工」（GetRewardMiners 的 SQL 是 " +
			"`epoch >= 起 and epoch < 止`），而清单模式每个高度各起一个同步器（批区间恒为 [h, h]）" +
			"⇒ 取矿工恒为空 ⇒ 跑完零写入（报告里仍显示全部高度完成）。请改用 --start/--end 区间模式")
	}
	return nil
}

// newRewardRepo 构造爆块奖励仓储。抽成包级变量只是为了让单测能注入假仓储
// （计算器要读 chain.miner_rewards / chain.miner_win_counts，离线的假连接无法应答那些 SQL），
// 生产路径始终是 dal.NewRewardTaskDal。
var newRewardRepo = func(db *gorm.DB) repository.RewardTask { return dal.NewRewardTaskDal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（chain），只注册计算器 calc-miner-agg-reward。
// SkipTraces = true：该计算器不读 traces，也不用聚合器与适配器。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.RewardTask = newRewardRepo(db)

	target := offlinereplay.Target{
		Name:       filscansyncer.ChainSyncer,
		SkipTraces: true,
	}

	if !noWrite {
		target.Calculators = []filscansyncer.Calculator{calc_miner_agg_reward.NewCalcMinerAggReward(repo)}
		return target, nil
	}

	wrapped := NewNoWriteRewardRepo(repo)
	target.Calculators = []filscansyncer.Calculator{calc_miner_agg_reward.NewCalcMinerAggReward(wrapped)}
	return target, wrapped.WriteStats
}
