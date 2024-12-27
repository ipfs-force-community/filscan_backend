package dal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewBaseFeeTrendBizDal(db *gorm.DB) *BaseFeeTrendBizDal {
	return &BaseFeeTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.BaseFeeTrendBizRepo = (*BaseFeeTrendBizDal)(nil)

type BaseFeeTrendBizDal struct {
	*_dal.BaseDal
}

func (b BaseFeeTrendBizDal) GetLatestStatBaseGasCostEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	err = b.Exec(ctx, func(tx *gorm.DB) error {
		table := po.BaseGasCostPo{}
		return tx.Table(table.TableName()).Select("epoch").Order("epoch desc").Limit(1).Scan(&epoch).Error
	})
	return
}

func (b BaseFeeTrendBizDal) GetStatBaseGasCost(ctx context.Context, points []chain.Epoch) (costs []*stat.BaseGasCost, err error) {
	err = b.Exec(ctx, func(tx *gorm.DB) error {
		
		var epochs []int64
		for _, v := range points {
			epochs = append(epochs, v.Int64())
		}
		var items []*po.BaseGasCostPo
		e := tx.Where("epoch in ?", epochs).Order("epoch desc").Find(&items).Error
		if e != nil {
			return e
		}
		c := convertor.BaseGasCostConvertor{}
		for _, v := range items {
			vv, ee := c.ToBaseGasCostEntity(v)
			if ee != nil {
				return ee
			}
			costs = append(costs, vv)
		}
		return nil
	})
	return
}
