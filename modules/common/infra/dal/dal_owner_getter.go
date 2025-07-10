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

func NewOwnerGetterDal(db *gorm.DB) *OwnerGetterDal {
	return &OwnerGetterDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.OwnerGetterRepo = (*OwnerGetterDal)(nil)

type OwnerGetterDal struct {
	*_dal.BaseDal
}

func (o OwnerGetterDal) IsOwner(ctx context.Context, addr chain.SmartAddress) (ok bool, err error) {

	tx, err := o.DB(ctx)
	if err != nil {
		return
	}

	var epoch po.SyncMinerEpochPo
	err = tx.Select("epoch").Order("epoch desc").First(&epoch).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
	}

	item := po.OwnerInfo{}
	err = tx.Select("owner").Where("epoch = ? and owner=?", epoch.Epoch, addr.Address()).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
	}

	ok = true

	return
}
