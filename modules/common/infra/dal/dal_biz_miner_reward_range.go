package dal

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

// 本文件是三个 londobell 统计端点的 PG 读实现（只读；不动同步器写路径）。
//
// 口径（对齐的是聚合器 **管线 JS 原文**，不是调用方猜测）：三个端点的 match 阶段一律是
//
//	Epoch: {$gte: ctx.StartEpoch, $lt: ctx.EndEpoch}
//
// ⇒ 区间是 **左闭右开 [start, end)**，SQL 里必须是 `epoch >= ? and epoch < ?`，
// 写成 `<= ?`（含右端点）会多出末端一个 epoch 的数据。
//
// 与聚合器逐字段的对应关系：
//
//	miner_blockreward  → 按 _id=$Epoch 分组：TotalBlockReward=Σ奖励消息 Value、BlockCount=出块数
//	miners_blockreward → 按 _id={Epoch,Miner} 分组：同上两个值
//	wincount           → 按 _id=Miner 分组：TotalWinCount=Σ WinCount、TotalGasReward=Σ GasReward
//
// 落库侧（同步器，同一批聚合结果的原样落盘）：
//
//	modules/syncer/chain/reward_task/reward_task.go:187-194  reward = TotalBlockReward（原样，无单位换算）
//	modules/syncer/chain/reward_task/reward_task.go:196-206  win_count = TotalWinCount、gas_reward = TotalGasReward（原样）
//
// ⇒ 读回来即可与本仓 pkg/londobell 的 decimal.Decimal 逐字段对齐。
//
// gas_reward（migration/36.miner_win_counts_gas_reward.sql 起）：
//
//	聚合器 wincount 端点的 TotalGasReward = Σ Message.Detail.Params.GasReward，
//	也就是 RewardActor.AwardBlockReward 隐式消息 params 的 GasReward 字段
//	（specs-actors v0.9.15 actors/builtin/reward/reward_actor.go:56）。它一直在同一次响应里，
//	此前只是 PO 没有列可落 ⇒ PG 路径该字段恒为 0，而它被 acl_block_chain.GetBlockDetails
//	用来算 TxFeeReward / MinedReward（assembler_block_chain_info.go:42-58）。
//
//	历史行的 gas_reward 是 NULL，由 ops/wincount_gas_reward/ 的回填脚本补齐；
//	未补齐前本 DAL 读出来的 sum 是残缺的，调用方用 total_rows/gas_reward_rows 判出来并回落聚合器。

// SQLMinerBlockRewardRange 单矿工逐 epoch 出块奖励。
//
// 去重口径：chain.miner_rewards 在 migration/1.chain.sql:159 有唯一索引 (epoch, miner)，
// 理论上不会出重复行；但写路径是纯 INSERT（CreateInBatches，见 dal_task_reward.go:122-136），
// 一旦线上唯一索引缺失/未生效（分区表的唯一索引只在建表脚本里声明过一次），重跑同一高度就会
// 产生完全相同的重复行。这里用 DISTINCT ON (epoch, miner) 兜住：重复行按 block_time 取最新一条，
// 保证「同 (epoch,miner) 只吐一行」，既不重不漏也不放大金额。
const SQLMinerBlockRewardRange = `
select epoch,
       reward      as reward,
       block_count as block_count
from (select distinct on (epoch, miner)
             epoch,
             miner,
             reward,
             block_count
      from chain.miner_rewards
      where miner = ?
        and epoch >= ?
        and epoch < ?
      order by epoch, miner, block_time desc nulls last) t
order by epoch asc`

// SQLMinersBlockRewardRange 逐 epoch 逐矿工出块奖励（去重口径同上）。
const SQLMinersBlockRewardRange = `
select epoch,
       miner,
       reward      as reward,
       block_count as block_count
from (select distinct on (epoch, miner)
             epoch,
             miner,
             reward,
             block_count
      from chain.miner_rewards
      where epoch >= ?
        and epoch < ?
      order by epoch, miner, block_time desc nulls last) t
order by epoch asc, miner asc`

// SQLMinerWinCountsRange 逐矿工 winCount 区间汇总。
//
// 去重口径（这张表**必须**去重）：migration/1.chain.sql:197-199 建的是**非唯一**索引，
// 且是 `on only`（不递归到分区），唯一性完全没有约束；写路径同样是纯 INSERT
// （dal_task_reward.go:69-80）。同一高度重跑同步 → 同一 (epoch, miner) 会有多行**完全相同**的
// win_count；若直接 sum(win_count) 会把该矿工的赢票数放大 N 倍（N=重跑次数）。
// 故先 DISTINCT ON (epoch, miner) 收敛成「每个 (epoch,miner) 一行」再按 miner 求和。
//
// gas_reward（migration/36 起）：与 win_count 同一份聚合器响应落库，故去重与求和口径完全一致。
// 额外的两列是**回填进度探针**，不是业务字段：
//
//	total_rows      = 去重后的行数
//	gas_reward_rows = 其中 gas_reward 非 NULL 的行数
//
// 只要 gas_reward_rows < total_rows，本区间的 sum(gas_reward) 就是**残缺的**
// （行数少的那些行是 migration/36 之前写入、尚未回填的），调用方必须回落聚合器 ——
// 详见 agg_pg_reward.go 的 incompleteGasRewardRow。
const SQLMinerWinCountsRange = `
select miner,
       sum(win_count)    as win_count,
       sum(gas_reward)   as gas_reward,
       count(*)          as total_rows,
       count(gas_reward) as gas_reward_rows
from (select distinct on (epoch, miner)
             epoch,
             miner,
             win_count,
             gas_reward
      from chain.miner_win_counts
      where epoch >= ?
        and epoch < ?
      order by epoch, miner) t
group by miner
order by miner asc`

func NewMinerRewardRangeDal(db *gorm.DB) *MinerRewardRangeDal {
	return &MinerRewardRangeDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MinerRewardRange = (*MinerRewardRangeDal)(nil)

// MinerRewardRangeDal 三个统计端点的 PG 读实现。
type MinerRewardRangeDal struct {
	*_dal.BaseDal
}

func (m MinerRewardRangeDal) MinerBlockRewardRange(ctx context.Context, miner string, start, end chain.Epoch) (items []*bo.MinerEpochReward, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(SQLMinerBlockRewardRange, miner, start.Int64(), end.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerRewardRangeDal) MinersBlockRewardRange(ctx context.Context, start, end chain.Epoch) (items []*bo.MinerEpochReward, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(SQLMinersBlockRewardRange, start.Int64(), end.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerRewardRangeDal) MinerWinCountsRange(ctx context.Context, start, end chain.Epoch) (items []*bo.AccWinCount, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(SQLMinerWinCountsRange, start.Int64(), end.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

// ===== 以下为 parity 工具（cmd/agg-parity）与运维自查用的只读诊断 SQL，不参与 API 读路径 =====

// SQLCountMinerRewardsRange 区间内 chain.miner_rewards 原始行数（含重复），用于判断「PG 一条都没有」
// 到底是覆盖缺口还是地址/区间口径问题。
const SQLCountMinerRewardsRange = `
select count(*) as cnt
from chain.miner_rewards
where epoch >= ?
  and epoch < ?`

// SQLCountMinerRewardsRangeByMiner 区间内指定矿工的原始行数（含重复）。
const SQLCountMinerRewardsRangeByMiner = `
select count(*) as cnt
from chain.miner_rewards
where miner = ?
  and epoch >= ?
  and epoch < ?`

// SQLDuplicateMinerRewards 判「同 (epoch,miner) 是否有重复行」；返回空 = 无重复（唯一索引生效）。
const SQLDuplicateMinerRewards = `
select epoch, miner, count(*) as cnt
from chain.miner_rewards
where epoch >= ?
  and epoch < ?
group by epoch, miner
having count(*) > 1
order by cnt desc, epoch asc
limit ?`

// SQLDuplicateMinerWinCounts 判 win_counts 的重复行（该表无非唯一约束，最可能出重复）。
const SQLDuplicateMinerWinCounts = `
select epoch, miner, count(*) as cnt
from chain.miner_win_counts
where epoch >= ?
  and epoch < ?
group by epoch, miner
having count(*) > 1
order by cnt desc, epoch asc
limit ?`

// SQLDistinctMinerInRewards 区间内出现过的 miner 值（用于核对地址前缀口径：库内应为带前缀的 f0…）。
const SQLDistinctMinerInRewards = `
select distinct miner
from chain.miner_rewards
where epoch >= ?
  and epoch < ?
order by miner asc
limit ?`

// SQLCountMinerWinCountsRange 区间内 chain.miner_win_counts 原始行数（含重复）。
const SQLCountMinerWinCountsRange = `
select count(*) as cnt
from chain.miner_win_counts
where epoch >= ?
  and epoch < ?`

// SQLTableRange 表内 epoch 覆盖范围（min/max）与行数——上线前确认覆盖是否到链头。
// min/max 用 coalesce 兜空表，避免调用方处理 NULL。
func SQLTableRange(table string) string {
	// 表名来自调用方的白名单常量（不是用户输入），此处只做拼接不做转义。
	return `select coalesce(min(epoch), 0) as min_epoch, coalesce(max(epoch), 0) as max_epoch, count(*) as cnt from ` + table
}
