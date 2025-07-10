package dal

import (
	"context"

	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewOwnerIndicatorDal(db *gorm.DB) *OwnerIndicatorBizDal {
	return &OwnerIndicatorBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type OwnerIndicatorBizDal struct {
	*_dal.BaseDal
}

func (o OwnerIndicatorBizDal) GetOwnerIndicator(ctx context.Context, ID actor.Id, interval *types.IntervalType) (item *bo.ActorIndicator, err error) {
	tx, err := o.DB(ctx)
	if err != nil {
		return
	}
	var epoch po.SyncMinerEpochPo
	err = tx.Select("epoch").Order("epoch desc").First(&epoch).Error
	if err != nil {
		return
	}

	var it string
	switch interval.Value() {
	case types.DAY:
		it = "24h"
	case types.WEEK:
		it = "7d"
	case types.MONTH:
		it = "30d"
	case types.YEAR:
		it = "365d"
	}

	err = tx.Debug().Raw(`
		select a.epoch,
	       a.owner,
	       a.sector_size * b.sector_count_change as seal_power_change,
	       a.quality_adj_power,
	       a.quality_adj_power_rank,
	       b.quality_adj_power_change,
	       b.acc_seal_gas,
	       b.acc_wd_post_gas,
	       b.acc_win_count,
	       b.acc_reward,
	       b.acc_block_count,
	       b.sector_count_change,
	       b.initial_pledge_change
	from chain.owner_infos a
	         left join chain.owner_stats b on a.epoch = b.epoch and a.owner = b.owner and b.interval = ?
	where a.epoch = ?
	  and a.owner = ?;
	`, it, epoch.Epoch, ID.Address()).
		Find(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	return
}
