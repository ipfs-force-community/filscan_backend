package minerrewardscmd

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// NoWriteRewardRepo 是 repository.RewardTask 的「只统计不落库」包装：
// 写方法（chain.miner_rewards / chain.owner_rewards / chain.miner_win_counts /
// chain.miner_reward_stats / chain.miner_agg_rewards 的增删）一律**不调用**底层仓储，只往计数器记账；
// 读方法（GetLastOwnerRewardOrNil / GetLastMinerRewardOrNil / ...）由嵌入接口原样透传。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使任务逻辑变化，
// 也不可能经由本类型写入上述任何一张派生表。
type NoWriteRewardRepo struct {
	// 嵌入底层仓储：读方法原样透传，只有写方法被本类型覆盖拦下
	repository.RewardTask

	minerRewards *offlinereplay.Counters // chain.miner_rewards
	ownerRewards *offlinereplay.Counters // chain.owner_rewards
	winCounts    *offlinereplay.Counters // chain.miner_win_counts
	rewardStats  *offlinereplay.Counters // chain.miner_reward_stats
	aggRewards   *offlinereplay.Counters // chain.miner_agg_rewards
}

var _ repository.RewardTask = (*NoWriteRewardRepo)(nil)

// 派生表名（与 po 的 TableName 一致）
const (
	TableMinerRewards     = "chain.miner_rewards"
	TableOwnerRewards     = "chain.owner_rewards"
	TableMinerWinCounts   = "chain.miner_win_counts"
	TableMinerRewardStats = "chain.miner_reward_stats"
	TableMinerAggRewards  = "chain.miner_agg_rewards"
)

// NewNoWriteRewardRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）
func NewNoWriteRewardRepo(inner repository.RewardTask) *NoWriteRewardRepo {
	if inner == nil {
		panic("minerrewardscmd: NewNoWriteRewardRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteRewardRepo{
		RewardTask:   inner,
		minerRewards: offlinereplay.NewCounters(TableMinerRewards),
		ownerRewards: offlinereplay.NewCounters(TableOwnerRewards),
		winCounts:    offlinereplay.NewCounters(TableMinerWinCounts),
		rewardStats:  offlinereplay.NewCounters(TableMinerRewardStats),
		aggRewards:   offlinereplay.NewCounters(TableMinerAggRewards),
	}
}

// SaveMinerRewards 拦下 chain.miner_rewards 写入，只计数（行数 = 本次本会写入的行数）
func (r *NoWriteRewardRepo) SaveMinerRewards(_ context.Context, rewards []*miner.Reward) error {
	r.minerRewards.CountWrite(len(rewards))
	return nil
}

// SaveOwnerRewards 拦下 chain.owner_rewards 写入，只计数
func (r *NoWriteRewardRepo) SaveOwnerRewards(_ context.Context, rewards []*owner.Reward) error {
	r.ownerRewards.CountWrite(len(rewards))
	return nil
}

// SaveWinCounts 拦下 chain.miner_win_counts 写入，只计数
func (r *NoWriteRewardRepo) SaveWinCounts(_ context.Context, winCounts []*po.MinerWinCount) error {
	r.winCounts.CountWrite(len(winCounts))
	return nil
}

// SaveMinerRewardStats 拦下 chain.miner_reward_stats 写入，只计数（本命令不跑该计算器，正常不会走到）
func (r *NoWriteRewardRepo) SaveMinerRewardStats(_ context.Context, stats []*po.MinerRewardStat) error {
	r.rewardStats.CountWrite(len(stats))
	return nil
}

// SaveMinerAggReward 拦下 chain.miner_agg_rewards 写入，只计数（本命令不跑该计算器，正常不会走到）
func (r *NoWriteRewardRepo) SaveMinerAggReward(_ context.Context, aggRewards []*po.MinerAggReward) error {
	r.aggRewards.CountWrite(len(aggRewards))
	return nil
}

// DeleteOwnerRewards 拦下 chain.owner_rewards 删除（RollBack 路径），只计数
func (r *NoWriteRewardRepo) DeleteOwnerRewards(_ context.Context, _ chain.Epoch) error {
	r.ownerRewards.CountDelete()
	return nil
}

// DeleteMinerRewards 拦下 chain.miner_rewards 删除（RollBack 路径），只计数
func (r *NoWriteRewardRepo) DeleteMinerRewards(_ context.Context, _ chain.Epoch) error {
	r.minerRewards.CountDelete()
	return nil
}

// DeleteWinCounts 拦下 chain.miner_win_counts 删除（RollBack 路径），只计数
func (r *NoWriteRewardRepo) DeleteWinCounts(_ context.Context, _ chain.Epoch) error {
	r.winCounts.CountDelete()
	return nil
}

// DeleteMinerRewardStats 拦下 chain.miner_reward_stats 删除（RollBack 路径），只计数
func (r *NoWriteRewardRepo) DeleteMinerRewardStats(_ context.Context, _ chain.Epoch) error {
	r.rewardStats.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteRewardRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{
		r.minerRewards.Snapshot(),
		r.ownerRewards.Snapshot(),
		r.winCounts.Snapshot(),
		r.rewardStats.Snapshot(),
		r.aggRewards.Snapshot(),
	}
}
