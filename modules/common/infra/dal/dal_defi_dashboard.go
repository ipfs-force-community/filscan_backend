package dal

import (
	"context"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

type DefiDashboardDal struct {
	*_dal.BaseDal
	repository.ERC20TokenRepo
}

func (d DefiDashboardDal) GetAllItemsOnEpoch(ctx context.Context, epoch int64) ([]*po.DefiDashboard, error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := []*po.DefiDashboard{}
	err = tx.Find(&res, "epoch = ?", epoch).Error
	return res, err
}

func (d DefiDashboardDal) GetItemsInRange(ctx context.Context, epoch int64) (int, []*po.DefiDashboard, error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return 0, nil, err
	}
	res := []*po.DefiDashboard{}

	err = tx.Find(&res, "epoch = ?", epoch).Error
	return len(res), res, err
}

func (d DefiDashboardDal) GetProductMainSite(ctx context.Context, productName string) string {
	tx, err := d.DB(ctx)
	if err != nil {
		return ""
	}
	res := po.ProductMainSite{}
	tx.First(&res, "product = ?", productName)
	return res.Url
}

func (d DefiDashboardDal) GetMaxHeight24hTvl(ctx context.Context) (tvl decimal.Decimal, tvl24h decimal.Decimal, err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	var epoch int64
	err = tx.Raw("select max(epoch) as ma from fevm.defi_dashboard").Scan(&epoch).Error
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	var res1, res2 decimal.Decimal
	err = tx.Raw("SELECT SUM(tvl_in_fil) FROM fevm.defi_dashboard WHERE epoch = ?", epoch).Scan(&res1).Error
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	err = tx.Raw("SELECT SUM(tvl_in_fil) FROM fevm.defi_dashboard WHERE epoch = ?", epoch-2880).Scan(&res2).Error
	return res1, res1.Sub(res2), err
}

func (d DefiDashboardDal) GetTvlByEpochs(ctx context.Context, epochs []chain.Epoch) ([]*bo.DefiTvl, error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return nil, err
	}
	var res = make([]*bo.DefiTvl, 0)
	//var res []decimal.Decimal
	err = tx.Raw("SELECT epoch, SUM(tvl_in_fil) as tvl FROM fevm.defi_dashboard WHERE epoch in ? group by  epoch", epochs).Scan(&res).Error

	return res, err
}

func (d DefiDashboardDal) GetMaxHeight(ctx context.Context) (int64, error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return 0, err
	}
	var res int64
	err = tx.Raw("select max(epoch) as ma from fevm.defi_dashboard").Scan(&res).Error
	return res, err
}

func (d DefiDashboardDal) CleanDefiItems(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return err
	}
	return tx.Delete(&po.DefiDashboard{}, "epoch >= ?", gteEpoch).Error
}

func (d DefiDashboardDal) BatchSaveDefiItems(ctx context.Context, items []*po.DefiDashboard) error {
	tx, err := d.DB(ctx)
	if err != nil {
		return err
	}
	return tx.CreateInBatches(items, 100).Error
}

func (d DefiDashboardDal) GetDefiItems(ctx context.Context, page, limit int) (int64, []*po.DefiDashboard, error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return 0, nil, err
	}
	max, count := new(int64), new(int64)
	err = tx.Model(&po.DefiDashboard{}).Select("max(epoch)").First(&max).Error
	if err != nil {
		return 0, nil, err
	}

	err = tx.Model(&po.DefiDashboard{}).Where("epoch = ?", max).Count(count).Error
	if err != nil {
		return 0, nil, err
	}
	out := []*po.DefiDashboard{}
	err = tx.Offset(limit*page).Limit(limit).Order("tvl desc").Find(&out, "epoch", max).Error

	return *count, out, err
}

func NewDefiDashboardDal(db *gorm.DB, repo repository.ERC20TokenRepo) *DefiDashboardDal {
	return &DefiDashboardDal{
		BaseDal:        _dal.NewBaseDal(db),
		ERC20TokenRepo: repo,
	}
}

var _ repository.DefiRepo = (*DefiDashboardDal)(nil)
