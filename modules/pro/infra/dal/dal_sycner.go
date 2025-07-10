package prodal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewSyncerDal(db *gorm.DB) *SyncerDal {
	return &SyncerDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.SyncerRepo = (*SyncerDal)(nil)

type SyncerDal struct {
	*_dal.BaseDal
}

func (s SyncerDal) MinerEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	var item *po.SyncSyncer
	err = tx.Where("name  = ?", syncer.MinerSyncer).First(&item).Error
	if err != nil {
		return
	}
	epoch = chain.Epoch(item.Epoch)
	return
}

func (s SyncerDal) TraceEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	var item *po.SyncSyncer
	err = tx.Where("name  = ?", syncer.ChainSyncer).First(&item).Error
	if err != nil {
		return
	}
	epoch = chain.Epoch(item.Epoch)
	return
}

func (s SyncerDal) SectorEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	var item *po.SyncSyncer
	err = tx.Where("name  = ?", "none").First(&item).Error
	if err != nil {
		return
	}
	epoch = chain.Epoch(item.Epoch)
	return
}

func (s SyncerDal) ChangeActorEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	var item *po.SyncSyncer
	err = tx.Where("name  = ?", syncer.ActorSyncer).First(&item).Error
	if err != nil {
		return
	}
	epoch = chain.Epoch(item.Epoch)
	return
}
