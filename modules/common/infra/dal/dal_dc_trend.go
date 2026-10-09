package dal

import (
	"context"

	"github.com/shopspring/decimal"
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

// dcRawRow 只承载链上真值（全网 raw / QA）；算力倍数口径由 biz 层经 chain.QualityTierSplit 派生，
// 本 DAL 不再做 DC/CC 或倍数拆分。
type dcRawRow struct {
	Epoch           int64           `gorm:"column:epoch"`
	RawBytePower    decimal.Decimal `gorm:"column:raw_byte_power"`
	QualityAdjPower decimal.Decimal `gorm:"column:quality_adj_power"`
}

func (d DcTrendDal) QueryDCPowers(ctx context.Context, epochs []int64) (items []*bo.DCPower, err error) {

	tx, err := d.DB(ctx)
	if err != nil {
		return
	}

	var rows []dcRawRow
	err = tx.Raw(`
		select epoch,
		       (state ->> 'TotalRawBytePower')::decimal    as raw_byte_power,
		       (state ->> 'TotalQualityAdjPower')::decimal as quality_adj_power
		from chain.builtin_actor_states
		where epoch in ?
		  and actor = 'f04'
		order by epoch desc`, epochs,
	).Find(&rows).Error
	if err != nil {
		return
	}

	for _, r := range rows {
		items = append(items, &bo.DCPower{
			Epoch:           r.Epoch,
			RawBytePower:    r.RawBytePower,
			QualityAdjPower: r.QualityAdjPower,
		})
	}

	return
}
