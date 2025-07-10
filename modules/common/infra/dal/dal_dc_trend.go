package dal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewDcTrendDal(db *gorm.DB) *DcTrendDal {
	return &DcTrendDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.StatisticDcTrendBizRepo = (*DcTrendDal)(nil)

type DcTrendDal struct {
	*_dal.BaseDal
}

func (d DcTrendDal) QueryDCPowers(ctx context.Context, epochs []int64) (items []*bo.DCPower, err error) {
	
	tx, err := d.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
		with a as (select epoch,
		                  (state ->> 'TotalRawBytePower')::decimal                                                       as rawBytePower,
		                  (((state ->> 'TotalQualityAdjPower')::decimal - (state ->> 'TotalRawBytePower')::decimal) / 9) as dc
		           from chain.builtin_actor_states
		           where epoch in ?
		             and actor = 'f04')
		select a.epoch,a.rawBytePower - a.dc as cc, a.dc
		from a order by epoch desc`, epochs,
	).Find(&items).Error
	if err != nil {
		return
	}
	
	return
}
