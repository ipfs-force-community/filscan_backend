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

func NewSyncEpochGetterDal(db *gorm.DB) *SyncEpochGetterDal {
	return &SyncEpochGetterDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.SyncEpochGetter = (*SyncEpochGetterDal)(nil)

type SyncEpochGetterDal struct {
	*_dal.BaseDal
}

func (s SyncEpochGetterDal) ChainEpoch(ctx context.Context) (epoch *chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	item := new(po.SyncEpochPo)
	err = tx.Select("epoch").Order("epoch desc").Limit(1).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	itemEpoch := chain.Epoch(item.Epoch)
	epoch = &itemEpoch
	return
}

func (s SyncEpochGetterDal) GetMinerEpochOrNil(ctx context.Context, epoch chain.Epoch) (result *chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	item := &po.SyncMinerEpochPo{}
	err = tx.Select("epoch").Where("epoch = ?", epoch.Int64()).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	t := chain.Epoch(item.Epoch)
	result = &t
	return
}

func (s SyncEpochGetterDal) MinerEpoch(ctx context.Context) (epoch *chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	item := new(po.SyncMinerEpochPo)
	err = tx.Select("epoch").Order("epoch desc").Limit(1).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	itemEpoch := chain.Epoch(item.Epoch)
	epoch = &itemEpoch
	return
}
