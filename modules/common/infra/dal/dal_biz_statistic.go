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

func (s StatisticBlockRewardTrendBizDal) GetBlockRewardsByEpochs(ctx context.Context, interval string, points []int64) (items []*bo.SumMinerReward, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	switch interval {
	case "1m":
		interval = "30d"
	}
	
	err = tx.Raw(`
		select b.epoch, b.balance, c.acc_reward_per_t
		from chain.builtin_actor_states as b
		         left join chain.miner_reward_stats c on b.epoch = c.epoch and c.interval = ?
		where b.epoch in ?
		  and b.actor = ?
		order by b.epoch desc;
	`, interval, points, builtin.RewardActorAddr.String(),
	).Find(&items).Error
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
