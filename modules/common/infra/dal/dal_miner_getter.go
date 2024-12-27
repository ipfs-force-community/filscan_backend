package dal

import (
	"context"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMinerGetterDal(db *gorm.DB) *MinerGetterDal {
	return &MinerGetterDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MinerGetterRepo = (*MinerGetterDal)(nil)
var _ repository.AbsPower = (*MinerGetterDal)(nil)

type MinerGetterDal struct {
	*_dal.BaseDal
}

func (o MinerGetterDal) GetMaxEpochInPowerAbs(ctx context.Context) (int64, error) {
	tx, err := o.DB(ctx)
	if err != nil {
		return 0, err
	}

	count := int64(0)

	err = tx.Model(&po.AbsPowerChange{}).Select("max(epoch)").Find(&count).Error
	return count, err
}

func (o MinerGetterDal) GetPowerAbs(ctx context.Context, start, end int64) ([]*po.AbsPowerChange, error) {
	tx, err := o.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := []*po.AbsPowerChange{}

	err = tx.Find(&res, "epoch >= ? and epoch <= ?", start, end).Error
	return res, err
}

func (o MinerGetterDal) IsMiner(ctx context.Context, addr chain.SmartAddress) (ok bool, err error) {

	tx, err := o.DB(ctx)
	if err != nil {
		return
	}

	var epoch po.SyncMinerEpochPo
	err = tx.Select("epoch").Order("epoch desc").First(&epoch).Error
	if err != nil {
		return
	}

	item := po.MinerInfo{}
	err = tx.Select("miner").Where("epoch = ? and miner=?", epoch.Epoch, addr.Address()).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
	}

	ok = true

	return
}
