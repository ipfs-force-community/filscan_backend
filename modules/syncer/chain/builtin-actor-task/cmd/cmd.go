// Package baselineactorscmd 提供「离线回放指定高度区间 / 高度清单的内置 Actor 状态派生数据」子命令。
//
// 用途（主网空洞回补）：把某段高度区间或某份高度清单的 chain.builtin_actor_states 补回来，
// 而**完全不碰**同步指针与台账（chain.sync_syncers / chain.sync_task_epochs /
// chain.sync_syncer_epochs / chain.sync_skipped_epochs 一个都不写）。
//
// 为什么「矿工/所有者统计」这条链需要它（本命令是这条链的**最底层输入**）：
// 空洞里 chain.builtin_actor_states 为空，会导致两张上游表按错误/零值算出来 ——
//
//	chain.miner_reward_stats.AccRewardPerT = 0
//	  （calc-miner-acc-reward-task → RewardTaskDal.GetNetQualityAdjPower，dal_task_reward.go:36-51，
//	    从 chain.builtin_actor_states 取 state->>'TotalQualityAdjPower'）
//	chain.miner_stats.luck_rate / wining_rate 失真
//	  （calc-miner-owner-task → luck.Calculator → LuckDal.GetNetQualityAjdPowerByPoints，
//	    dal_luck.go:25-46，同样读 chain.builtin_actor_states 的 ThisEpochQualityAdjPower）
//
// 目标表：chain.builtin_actor_states（UNIQUE (epoch, actor)，migration/1.chain.sql:39）
// —— 每个高度写 2 行：reward actor 与 storage power actor 的状态 JSON。
//
// 对应生产任务：chain 同步器第 1 个任务组的 baseline-task
// （modules/syncer/chain/builtin-actor-task，Task.Name() = "baseline-task"；生产装配见
// injector/syncer_manager.go:170）。它是**同步任务**：对高度连续性没有要求，但**每个高度都写**
// （只受 ctx.Empty() 限制）。
//
// 触发条件：每个高度执行；ctx.Empty() 时直接返回，不写任何表。
//
// 输入（决定「它依赖谁」）：
//  1. 适配器 adapter.Actor(RewardActorAddr, epoch)（builtin_actor.go:92）—— 外部数据源，**不在空洞内**；
//  2. 适配器 adapter.Actor(StoragePowerActorAddr, epoch)（builtin_actor.go:112）—— 外部数据源，**不在空洞内**。
//
// ⇒ **本任务的全部输入都是「节点侧按高度回溯取状态」，不依赖任何数据库表** ⇒ 在空洞里可以
// **安全回放**：不存在「输入也是空的」的递归依赖。（这也是它能作为整条链最底层的原因。）
//
// 输入为空会导致哪些列偏低：适配器取不到该高度的 actor 状态 ⇒ 任务直接报错（builtin_actor.go:50-58），
// 该高度按保险丝重试后被放弃并列入报告（**不会**写入半截数据）；某个高度缺行 ⇒ 依赖它的
// AccRewardPerT / luck_rate **偏低（或为 0）**。
//
// 是否读 traces：**不读**（也没有数据库输入）⇒ Target.SkipTraces = true，
// 省掉每个高度一次 Traces 调用。
//
// 幂等性 / 重跑：chain.builtin_actor_states 有唯一键 (epoch, actor) ⇒ 重跑同一高度会**撞唯一键**
// （该高度重试后被保险丝放弃并列进报告）。重跑前先按高度删旧行：
//
//	delete from chain.builtin_actor_states where epoch = <高度>;
package baselineactorscmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/builtin-actor-task"
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

// Command 离线回放内置 Actor 状态派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "baseline-actors [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的内置 Actor 状态（补 chain.builtin_actor_states）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的内置 Actor 状态，补 chain.builtin_actor_states（每高度 2 行：reward / power actor）。\n" +
			"只跑 sync task baseline-task（生产 chain 同步器 1 号任务组的那个任务）。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续空洞用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"为什么要它（本命令是矿工/所有者统计链的最底层输入）：chain.miner_reward_stats.AccRewardPerT\n" +
			"与 chain.miner_stats.luck_rate 都从 chain.builtin_actor_states 取全网算力 ——\n" +
			"该表为空时这两个指标会算成 0 / 失真。本任务的输入只有节点侧按高度回溯的 actor 状态，\n" +
			"**不依赖任何数据库表** ⇒ 空洞里可安全回放。\n\n" +
			"幂等性：chain.builtin_actor_states 有唯一键 (epoch, actor)，重跑同一高度会撞唯一键。\n" +
			"重跑前先按高度删除旧行。\n\n" +
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

// newBaselineRepo 构造内置 Actor 状态仓储。抽成包级变量只是为了让单测能注入假仓储
// （离线的假连接无法应答 SQL），生产路径始终是 dal.NewBaseLineTaskDal。
var newBaselineRepo = func(db *gorm.DB) repository.BaselineTaskRepo { return dal.NewBaseLineTaskDal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（chain），只注册 sync task baseline-task。
// SkipTraces = true：该任务不读 traces（也没有数据库输入，只用适配器）。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.BaselineTaskRepo = newBaselineRepo(db)

	target := offlinereplay.Target{
		Name:       filscansyncer.ChainSyncer,
		SkipTraces: true,
	}

	if !noWrite {
		target.Groups = []filscansyncer.TaskGroup{{builtin_actor_task.NewBaselineTask(repo)}}
		return target, nil
	}

	wrapped := NewNoWriteBaselineRepo(repo)
	target.Groups = []filscansyncer.TaskGroup{{builtin_actor_task.NewBaselineTask(wrapped)}}
	return target, wrapped.WriteStats
}
