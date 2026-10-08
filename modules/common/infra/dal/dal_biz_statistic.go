package dal

import (
	"context"

	"github.com/filecoin-project/go-state-types/builtin"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewStatisticBlockRewardTrendBizDal(db *gorm.DB) *StatisticBlockRewardTrendBizDal {
	return &StatisticBlockRewardTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.StatisticBlockRewardTrendBizRepo = (*StatisticBlockRewardTrendBizDal)(nil)

type StatisticBlockRewardTrendBizDal struct {
	*_dal.BaseDal
}

// SQLBlockRewardTrend 是统计页「累计区块奖励」曲线的唯一取数事实源。
//
// 口径（契约 A1）：曲线含义 = 「累计发放给矿工的区块奖励」，**不是** f02 累计铸造量：
//   - v18 及以前：state.TotalStoragePowerReward 本身就是矿工份额，直接用；
//   - NV29(Solstice) 起：state 里是 TotalMintedReward/TotalBurnMinted/TotalExplicitMinted，
//     矿工份额 = TotalMintedReward − TotalBurnMinted − TotalExplicitMinted
//     （对齐 lotus streams.go 的 MinerMinted()，与 pkg/londobell.minerMinted 同一口径）。
//
// 分支判断为什么用「数值非零」而不是契约草稿里的 `->> 'TotalMintedReward' is not null`：
// chain.builtin_actor_states.state 落库的是 Go 结构体 londobell.RewardActorState 的 json.Marshal
// （builtin-actor-task/builtin_actor.go:60-68），结构体里 TotalMintedReward 无 omitempty，
// 于是 **v18 行也会带 `"TotalMintedReward":"0"`** —— PG 实测 `->> is not null` 对这类真实
// v18 行返回 false（值存在但为 "0"），若按空值判断就会走进相减分支、把整条曲线算成 0。
// 故以「值非零」为准：v18 行 → 走 TotalStoragePowerReward；v19 行（TotalMintedReward 非零）
// → 相减；补丁前写入的 v19 行（TotalMintedReward 缺失/NULL）→ coalesce 归 0 → 回退 legacy。
//
// 不再使用 `1.1e9 FIL − f02 账户余额`：那条式子等于 f02 累计出账，NV29 后含服务流+销毁，
// 已与「矿工实收」口径脱钩（Cali 实测偏高 17.9 万 FIL 且日增 3.2 万）。
const SQLBlockRewardTrend = `
	select b.epoch,
	       case when coalesce((b.state ->> 'TotalMintedReward')::numeric, 0) <> 0
	            then (b.state ->> 'TotalMintedReward')::numeric
	                 - coalesce((b.state ->> 'TotalBurnMinted')::numeric, 0)
	                 - coalesce((b.state ->> 'TotalExplicitMinted')::numeric, 0)
	            else coalesce((b.state ->> 'TotalStoragePowerReward')::numeric, 0) end as acc_block_rewards,
	       c.acc_reward_per_t
	from chain.builtin_actor_states as b
	         left join chain.miner_reward_stats c on b.epoch = c.epoch and c.interval = ?
	where b.epoch in ?
	  and b.actor = ?
	order by b.epoch desc;
`

func (s StatisticBlockRewardTrendBizDal) GetBlockRewardsByEpochs(ctx context.Context, interval string, points []int64) (items []*bo.SumMinerReward, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	switch interval {
	case "1m":
		interval = "30d"
	}

	err = tx.Raw(SQLBlockRewardTrend, interval, points, builtin.RewardActorAddr.String()).Find(&items).Error
	if err != nil {
		return
	}

	return
}

func NewStatisticActiveMinerTrendBizDal(db *gorm.DB) *StatisticActiveMinerTrendBizDal {
	return &StatisticActiveMinerTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.StatisticActiveMinerTrendBizRepo = (*StatisticActiveMinerTrendBizDal)(nil)

type StatisticActiveMinerTrendBizDal struct {
	*_dal.BaseDal
}

func (s StatisticActiveMinerTrendBizDal) GetActiveMinerCountsByEpochs(ctx context.Context, epochs []chain.Epoch) (items []*bo.ActiveMinerCount, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	var points []int64
	for _, v := range epochs {
		points = append(points, v.Int64())
	}

	err = tx.Raw(`select epoch, cast(state ->> 'MinerAboveMinPowerCount' as bigint) as active_miners
		from chain.builtin_actor_states
		where epoch in ?
		  and actor = ?`,
		points, builtin.StoragePowerActorAddr.String()).Find(&items).Error
	if err != nil {
		return
	}

	return
}

func NewStatisticMessageCountTrendBizDal(db *gorm.DB) *StatisticMessageCountTrendBizDal {
	return &StatisticMessageCountTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.StatisticMessageCountTrendBizRepo = (*StatisticMessageCountTrendBizDal)(nil)

type StatisticMessageCountTrendBizDal struct {
	*_dal.BaseDal
}

func (s StatisticMessageCountTrendBizDal) GetMessageCountsByEpochs(ctx context.Context, points []chain.Epoch) (items []*bo.MessageCount, err error) {

	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	var epochs []int64
	for _, v := range points {
		epochs = append(epochs, v.Int64())
	}

	err = tx.Raw(`select epoch,avg_block_message
		from chain.message_counts
		where epoch in (?)
		order by epoch desc`, epochs).Find(&items).Error
	if err != nil {
		return
	}

	return
}
