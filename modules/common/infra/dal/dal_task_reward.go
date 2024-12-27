package dal

import (
	"context"
	
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewRewardTaskDal(db *gorm.DB) *RewardTaskDal {
	return &RewardTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.RewardTask = (*RewardTaskDal)(nil)

type RewardTaskDal struct {
	*_dal.BaseDal
}

func (m RewardTaskDal) SaveMinerRewardStats(ctx context.Context, stats []*po.MinerRewardStat) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(stats, 5).Error
	})
	return
}

func (m RewardTaskDal) GetNetQualityAdjPower(ctx context.Context, epoch chain.Epoch) (power decimal.Decimal, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`
		select cast(state ->> 'TotalQualityAdjPower' as decimal) as quality_adj_power
		from chain.builtin_actor_states
		where epoch = ?
		  and actor = ?
	`, epoch.Int64(), builtin.StoragePowerActorAddr.String()).Scan(&power).Error
	if err != nil {
		return
	}
	return
}

func (m RewardTaskDal) SumRewards(ctx context.Context, epochs chain.LCRCRange) (value decimal.Decimal, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
	select greatest(sum(reward), 0) from chain.miner_rewards where epoch >= ? and epoch <= ?
	`, epochs.GteBegin.Int64(), epochs.LteEnd.Int64()).Scan(&value).Error
	if err != nil {
		return
	}
	
	return
}

func (m RewardTaskDal) SaveWinCounts(ctx context.Context, winCounts []*po.MinerWinCount) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(winCounts, 5).Error
	if err != nil {
		return
	}
	
	return
}

func (m RewardTaskDal) GetLastMinerRewardOrNil(ctx context.Context, epoch chain.Epoch, miner chain.SmartAddress) (entity *miner.Reward, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	item := new(po.MinerRewardPo)
	err = tx.Where("epoch < ? and miner= ?", epoch.Int64(), miner.Address()).Order("epoch desc").First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	
	c := convertor.MinerRewardConvertor{}
	entity, err = c.ToMinerRewardEntity(item)
	
	return
}

func (m RewardTaskDal) GetLastOwnerRewardOrNil(ctx context.Context, epoch chain.Epoch, owner chain.SmartAddress) (entity *owner.Reward, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	item := new(po.OwnerRewardPo)
	err = tx.Where("epoch < ? and owner= ?", epoch.Int64(), owner.Address()).Order("epoch desc").First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	
	c := convertor.OwnerRewardConvertor{}
	entity, err = c.ToOwnerRewardEntity(item)
	
	return
}

func (m RewardTaskDal) SaveMinerRewards(ctx context.Context, rewards []*miner.Reward) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		c := convertor.MinerRewardConvertor{}
		var items []*po.MinerRewardPo
		for _, v := range rewards {
			item, e := c.ToMinerRewardPo(v)
			if err != nil {
				return e
			}
			items = append(items, item)
		}
		return tx.CreateInBatches(items, 5).Error
	})
	return
}

func (m RewardTaskDal) SaveOwnerRewards(ctx context.Context, rewards []*owner.Reward) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	var items []*po.OwnerRewardPo
	c := convertor.OwnerRewardConvertor{}
	for _, v := range rewards {
		var item *po.OwnerRewardPo
		item, err = c.ToOwnerRewardPo(v)
		if err != nil {
			return
		}
		items = append(items, item)
	}
	
	return tx.CreateInBatches(items, 5).Error
}

func (m RewardTaskDal) DeleteOwnerRewards(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_rewards where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m RewardTaskDal) DeleteMinerRewards(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_rewards where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m RewardTaskDal) DeleteWinCounts(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_win_counts where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m RewardTaskDal) DeleteMinerRewardStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_reward_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m RewardTaskDal) GetRewardMiners(ctx context.Context, epochs chain.LCRCRange) (miners []string, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`
		select distinct miner from chain.miner_rewards where epoch >= ? and epoch < ?`,
		epochs.GteBegin.Int64(),
		epochs.LteEnd.Int64(),
	).Scan(&miners).Error
	return
}

func (m RewardTaskDal) SumMinersTotalRewards(ctx context.Context, miners []string) (aggRewards []*po.MinerAggReward, err error) {
	
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
		select miner, sum(reward) as agg_reward, sum(block_count) as agg_block_count
		from chain.miner_rewards
		where miner in ?
		group by miner;
	`, miners).Find(&aggRewards).Error
	if err != nil {
		return
	}
	
	var winCounts []*po.MinerAggReward
	err = tx.Raw(`
		select miner, sum(win_count) as agg_win_count
		from chain.miner_win_counts
		where miner in ?
		group by miner
	`, miners).Find(&winCounts).Error
	if err != nil {
		return
	}
	winCountsMap := map[string]int64{}
	for _, v := range winCounts {
		winCountsMap[v.Miner] = v.AggWinCount
	}
	
	for _, v := range aggRewards {
		if vv, ok := winCountsMap[v.Miner]; ok {
			v.AggWinCount = vv
		}
	}
	
	return
}

func (m RewardTaskDal) SaveMinerAggReward(ctx context.Context, aggRewards []*po.MinerAggReward) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	for _, v := range aggRewards {
		err = tx.Exec(`
			insert into chain.miner_agg_rewards(miner, agg_reward, agg_block, agg_win_count)
			VALUES (?, ?, ?, ?)
			on conflict (miner) do update set agg_reward    = excluded.agg_reward,
			                                  agg_block     = excluded.agg_block,
			                                  agg_win_count = excluded.agg_win_count;
          `, v.Miner, v.AggReward, v.AggBlockCount, v.AggWinCount).Error
		if err != nil {
			return
		}
	}
	
	return
}
