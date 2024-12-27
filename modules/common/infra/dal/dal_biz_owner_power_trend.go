package dal

import (
	"context"
	
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewOwnerPowerTrendBizDal(db *gorm.DB) *OwnerPowerTrendBizDal {
	return &OwnerPowerTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type OwnerPowerTrendBizDal struct {
	*_dal.BaseDal
}

func (m OwnerPowerTrendBizDal) GetOwnerPowerTrend(ctx context.Context, points []chain.Epoch, ownerID actor.Id) (ownerPowerTrend []*bo.ActorPowerTrend, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	var epochs []int64
	for _, epoch := range points {
		epochs = append(epochs, epoch.Int64())
	}
	
	err = tx.Raw(`
				select a.epoch,a.owner as account_id, a.quality_adj_power as power, (a.quality_adj_power - b.quality_adj_power ) as power_change
			from chain.owner_infos a
			         left join chain.owner_infos b on b.epoch = a.epoch - 120 and a.owner = b.owner
			where a.epoch in ?
			  and a.owner = ?
			order by a.epoch desc;
		`, epochs, ownerID.Address()).Find(&ownerPowerTrend).Error
	if err != nil {
		return
	}
	
	for _, v := range ownerPowerTrend {
		v.BlockTime = chain.Epoch(v.Epoch).Unix()
	}
	
	return
}
