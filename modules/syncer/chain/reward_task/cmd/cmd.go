// Package minerrewardscmd 提供「离线回放指定高度区间 / 高度清单的矿工爆块奖励派生数据」子命令。
//
// 用途（主网空洞回补）：把某段高度区间或某份高度清单的矿工（及所有者）爆块奖励补回来，
// 而**完全不碰**同步指针与台账（chain.sync_syncers / chain.sync_task_epochs /
// chain.sync_syncer_epochs / chain.sync_skipped_epochs 一个都不写）。
//
// 目标表（三张，唯一键情况各不相同 —— 直接决定「能不能重跑」）：
//
//	chain.miner_rewards    —— UNIQUE (epoch, miner)     （migration/1.chain.sql:160）
//	chain.owner_rewards    —— UNIQUE (epoch, owner)     （migration/1.chain.sql:245）
//	chain.miner_win_counts —— **无唯一键**，只有 (epoch,miner) / (miner,epoch) 普通索引（migration/1.chain.sql:197-199）
//
// 对应生产任务：chain 同步器第 1 个任务组的 reward_task
// （modules/syncer/chain/reward_task，Task.Name() = "reward-task"；生产装配见
// injector/syncer_manager.go:171）。它是**同步任务**（不是计算器）：对高度连续性没有要求，
// 但**每个高度都写**（只受 ctx.Empty() 限制：空高度不写）。
//
// 触发条件：每个高度执行；ctx.Empty()（该高度没有 tipset）时直接返回，不写任何表。
//
// 输入（决定「它依赖谁」）：
//  1. 聚合器 agg.MinersBlockReward(epoch, epoch+1) 与 agg.WinCount(epoch, epoch+1)
//     （reward_task.go:82/87）—— 外部数据源，**不在空洞内**；
//  2. 适配器 adapter.Miner(miner, epoch) 取该矿工在该高度的 owner（reward_task.go:125）—— 外部；
//  3. chain.owner_rewards 里「同一 owner、epoch < 当前高度」的**上一条**记录
//     （repo.GetLastOwnerRewardOrNil，dal_task_reward.go:102-120），用于累计口径
//     acc_reward / acc_block_count / prev_epoch_ref。
//
// 输入为空会导致哪些列偏低（务必先读）：
//   - chain.owner_rewards 的上一条记录取不到（例如只补空洞中段、没补它前面的高度，或没按升序补），
//     则该 owner 的 acc_reward / acc_block_count 会**从零重新累计**（偏低）、prev_epoch_ref 指向错误高度。
//     ⇒ 同一个 owner 的历史高度必须**按升序**、从空洞起点开始连续补；空洞起点之前的高度本来就已存在。
//   - 聚合器 MinersBlockReward / WinCount 取不到该高度（返回空）⇒ chain.miner_rewards 与
//     chain.miner_win_counts 在该高度整段缺失（不是偏低，是没有行）。
//   - adapter.Miner 取不到 owner ⇒ 该矿工的 owner_rewards **整段缺失**（reward_task 会直接返回错误，
//     该高度按保险丝重试后被放弃并列入报告）。
//
// 是否读 traces：**不读**（reward_task 只查聚合器与适配器，没有任何 Datamap().Get）⇒
// Target.SkipTraces = true，与 actor 同步器同理：省掉每个高度一次 Traces 调用，
// 也避免 traces 取不到时整个高度失败。
//
// 幂等性 / 重跑：
//
//   - chain.miner_rewards / chain.owner_rewards 有唯一键 ⇒ 同一高度跑第二遍会**撞唯一键**
//     （该高度重试后被保险丝放弃并列进报告）。重跑前先按高度删旧行：
//
//     delete from chain.miner_rewards where epoch = <高度>;
//     delete from chain.owner_rewards where epoch = <高度>;
//
//   - chain.miner_win_counts **没有唯一键** ⇒ 重跑会**静默追加重复行**（不报错！），
//     后果是该高度的总赢票被多算（luck_rate / wining_rate 会跟着偏高）。重跑前必须删：
//
//     delete from chain.miner_win_counts where epoch = <高度>;
//
// 上游依赖（谁会因为本表为空而偏低）：chain.miner_stats / chain.owner_stats
// （calc-miner-owner-task 读 GetMinersAccRewards / GetMinersAccWinCount）、
// chain.miner_reward_stats（calc-miner-acc-reward-task 读 SumRewards）、
// chain.miner_agg_rewards（calc-miner-agg-reward）。⇒ 本命令必须排在它们**之前**。
//
// 已知偏差（既有实现，本命令**忠实复现**、不做修正 —— 改它等于改生产 chain 同步器的口径，
// 需要业务先点头）：chain/reward_task/reward_task.go:159-168 把「上一条 owner_rewards」
// 加在**每个爆块矿工**的循环里，于是同一高度同一 owner 下有 N 个爆块矿工时，上一条会被累加 N 次
// ⇒ chain.owner_rewards 的 acc_reward / acc_block_count **偏高**（本高度的 reward / block_count 是对的）。
// 回补空洞时这个偏差会同样成立：想拿到「准的累计值」必须先修 reward_task，而不是靠回放。
package minerrewardscmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/reward_task"
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

// Command 离线回放矿工爆块奖励派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "miner-rewards [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的矿工爆块奖励（补 chain.miner_rewards / owner_rewards / miner_win_counts）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的矿工爆块奖励，补 chain.miner_rewards / chain.owner_rewards / chain.miner_win_counts\n" +
			"（只跑 sync task reward-task，即生产 chain 同步器 1 号任务组的那个任务）。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续空洞用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"输入与偏差（跑之前务必先读）：\n" +
			"  1. chain.owner_rewards 的累计口径依赖「同一 owner 的上一条记录」——\n" +
			"     若它取不到，acc_reward / acc_block_count 会从零重新累计（偏低）；\n" +
			"     ⇒ 同一 owner 的历史高度必须按升序、从空洞起点连续补。\n" +
			"  2. 聚合器在该高度返回空 ⇒ 该高度的 chain.miner_rewards / miner_win_counts 整段缺失。\n" +
			"  3. 本任务不读 traces（无 SetTracesBuilder，也省掉每高度一次 Traces 调用）。\n\n" +
			"幂等性：chain.miner_rewards(epoch,miner) 与 chain.owner_rewards(epoch,owner) 有唯一键，\n" +
			"重跑同一高度会撞唯一键；chain.miner_win_counts **没有唯一键**，重跑会静默追加重复行（赢票被多算）。\n" +
			"重跑前先按高度删除这三张表的旧行。\n\n" +
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
// 返回值：计划/依赖装配失败才返回错误；单个高度失败不返回错误（记进报告，见框架注释）。
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

	target, writes := buildTarget(deps.DB, option.noWrite)

	opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
	opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

	return offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
}

// resolvePlan 计划解析的唯一入口（单独抽出来，单测可直接覆盖参数校验：Run 里非法参数会 log.Fatal）
func resolvePlan(option options) (offlinereplay.Plan, error) {
	return offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
}

// newRewardRepo 构造爆块奖励仓储。抽成包级变量只是为了让单测能注入假仓储
// （reward-task 要读 chain.owner_rewards 的上一行，离线的假连接无法应答该 SQL），
// 生产路径始终是 dal.NewRewardTaskDal。
var newRewardRepo = func(db *gorm.DB) repository.RewardTask { return dal.NewRewardTaskDal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（chain），只注册 sync task reward-task。
// 与生产 chain 同步器一致地不注入 traces（reward-task 不读 Datamap 里的 traces）：
// 于是省掉每个高度一次 Traces 调用，也避免 traces 取不到时整个高度失败。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.RewardTask = newRewardRepo(db)

	target := offlinereplay.Target{
		Name:       filscansyncer.ChainSyncer,
		SkipTraces: true,
	}

	if !noWrite {
		target.Groups = []filscansyncer.TaskGroup{{reward_task.NewMinerRewardTask(repo)}}
		return target, nil
	}

	wrapped := NewNoWriteRewardRepo(repo)
	target.Groups = []filscansyncer.TaskGroup{{reward_task.NewMinerRewardTask(wrapped)}}
	return target, wrapped.WriteStats
}
