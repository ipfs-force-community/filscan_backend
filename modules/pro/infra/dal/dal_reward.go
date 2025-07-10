package prodal

import (
	"context"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewRewardDal(db *gorm.DB) *RewardDal {
	return &RewardDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.RewardRepo = (*RewardDal)(nil)

type RewardDal struct {
	*_dal.BaseDal
}

func (r RewardDal) GetMinerRewards(ctx context.Context, miners []string, epochs chain.LCRORange) (items []prorepo.MinerReward, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`
with a as (select miner, sum(reward) as reward, sum(block_count) as block_count
           from chain.miner_rewards
           where epoch < ?
             and epoch >= ?
			 and miner in ?
           group by miner)
select a.*, b.win_count, d.acc_reward
from a
         full join (select miner, sum(win_count) as win_count
                    from chain.miner_win_counts
                    where epoch < ?
		             and epoch >= ?
					 and miner in ?
                    group by miner) b on a.miner = b.miner
         left join (select *
                    from (select epoch, miner, acc_reward, rank() over (partition by miner order by epoch desc ) as rnk
                          from chain.miner_rewards
                          where epoch < ?
			              and epoch >= ?
						  and miner in ?) c
                    where c.rnk = 1) d on a.miner = d.miner
order by miner desc;
       `, epochs.LtEnd.Int64(),
		epochs.GteBegin.Int64(),
		miners,
		epochs.LtEnd.Int64(),
		epochs.GteBegin.Int64(),
		miners,
		epochs.LtEnd.Int64(),
		epochs.GteBegin.Int64(),
		miners,
	).Find(&items).Error
	if err != nil {
		return
	}
	return
}
