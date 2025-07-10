package dal

import (
	"context"
	"time"

	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

var _ repository.FilPriceRepo = (*FilPriceDal)(nil)

type FilPriceDal struct {
	*_dal.BaseDal
}

func (f FilPriceDal) SaveFilPrice(ctx context.Context, price, percentChange float64, time time.Time) error {
	tx, err := f.DB(ctx)
	if err != nil {
		return err
	}
	return tx.Save(&po.FilPrice{
		Price:         price,
		PercentChange: percentChange,
		Timestamp:     time.Unix(),
	}).Error
}

func (f FilPriceDal) LatestPrice(ctx context.Context) (*po.FilPrice, error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := po.FilPrice{}
	err = tx.Order("id DESC").First(&res).Error
	return &res, err
}

func NewFilPriceDal(db *gorm.DB) *FilPriceDal {
	return &FilPriceDal{BaseDal: _dal.NewBaseDal(db)}
}
