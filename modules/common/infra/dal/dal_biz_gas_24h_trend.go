package dal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewGas24hTrendBizDal(db *gorm.DB) *Gas24hTrendBizDal {
	return &Gas24hTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.Gas24hTrendBizRepo = (*Gas24hTrendBizDal)(nil)

type Gas24hTrendBizDal struct {
	*_dal.BaseDal
}

func (g Gas24hTrendBizDal) GetLatestMethodGasCostEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	
	err = g.Exec(ctx, func(tx *gorm.DB) error {
		table := po.MethodGasFee{}
		return tx.Table(table.TableName()).Select("epoch").Order("epoch desc").Limit(1).Scan(&epoch).Error
	})
	return
}

func (g Gas24hTrendBizDal) GetMethodGasFees(ctx context.Context, epochs chain.LCRORange) (costs []*po.MethodGasFee, err error) {
	db, err := g.DB(ctx)
	if err != nil {
		return
	}
	
	err = db.Raw(`
			select method,
		       sum(count)       as count,
		       sum(gas_premium) as gas_premium,
		       sum(gas_limit)   as gas_limit,
		       sum(gas_cost)    as gas_cost,
		       sum(gas_fee)     as gas_fee
		from chain.method_gas_fees
		where epoch > ?
		  and epoch <= ?
		group by method having sum(gas_cost) > 0
		order by gas_fee desc
	`, epochs.GteBegin.Int64(), epochs.LtEnd.Int64()).Find(&costs).Error
	if err != nil {
		return
	}
	
	return
}
