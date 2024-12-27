package dal

import (
	"context"

	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewSyncerDal(db *gorm.DB) *SyncerDal {
	return &SyncerDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.SyncerRepo = (*SyncerDal)(nil)

type SyncerDal struct {
	*_dal.BaseDal
}

func (s SyncerDal) GetSyncer(ctx context.Context, syncer string) (item *po.SyncSyncer, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}

	item = new(po.SyncSyncer)
	err = db.Where("name = ?", syncer).First(item).Error
	if err != nil {
		return
	}

	return
}

func (s SyncerDal) DeleteSyncTaskEpochs(ctx context.Context, gteEpoch chain.Epoch, syncer string) (err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = db.Exec("delete from chain.sync_task_epochs where epoch >= ? and syncer = ?", gteEpoch, syncer).Error
	if err != nil {
		return
	}
	return
}

func (s SyncerDal) DeleteSyncSyncerEpochs(ctx context.Context, gteEpoch chain.Epoch, name string) (err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = db.Exec("delete from chain.sync_syncer_epochs where epoch >= ? and name = ?", gteEpoch, name).Error
	if err != nil {
		return
	}
	return
}

func (s SyncerDal) SaveSyncer(ctx context.Context, task *po.SyncSyncer) (err error) {
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		item := new(po.SyncSyncer)
		if e := tx.Where("name=?", task.Name).First(item).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				item = nil
				e = nil
			} else {
				return e
			}
		}
		if item != nil {
			return tx.Table(item.TableName()).Where("name=?", task.Name).Update("epoch", task.Epoch).Error
		}

		return tx.Create(task).Error
	})
	return
}

func (s SyncerDal) GetSyncerOrNil(ctx context.Context, name string) (syncer *po.SyncSyncer, err error) {
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		syncer = new(po.SyncSyncer)
		e := tx.Where("name=?", name).First(syncer).Error
		if e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				e = nil
				syncer = nil
			}
			return e
		}
		return nil
	})
	return
}

func (s SyncerDal) SaveSyncSyncerEpoch(ctx context.Context, item *po.SyncSyncerEpoch) (err error) {
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "epoch"},
				{Name: "name"},
			},
			DoNothing: true}).Create(item).Error
	})
	return
}

func (s SyncerDal) GetSyncSyncerEpochOrNil(ctx context.Context, epoch chain.Epoch, name string) (item *po.SyncSyncerEpoch, err error) {

	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	item = new(po.SyncSyncerEpoch)
	err = tx.Where("epoch=? and name=?", epoch.Int64(), name).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}

	return
}

func (s SyncerDal) SaveSyncTaskEpochEpoch(ctx context.Context, item *po.SyncTaskEpoch) (err error) {
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Create(item).Error
	})
	return
}

func (s SyncerDal) GetLatestSyncSyncerEpochOrNil(ctx context.Context, name string) (item *po.SyncSyncerEpoch, err error) {

	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	item = new(po.SyncSyncerEpoch)
	err = tx.Where("name = ?", name).Order("epoch desc").First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}

	return
}

func (s SyncerDal) GetSyncTaskEpochOrNil(ctx context.Context, epoch chain.Epoch, name string) (item *po.SyncTaskEpoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	item = new(po.SyncTaskEpoch)
	err = tx.Where("epoch = ? and task = ?", epoch.Int64(), name).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}

	return
}

func (s SyncerDal) GetSyncSyncerEpochs(ctx context.Context, name string, epochs chain.LCRCRange) (items []*po.SyncSyncerEpoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where("epoch >= ? and epoch <= ? and name = ? and empty is false",
		epochs.GteBegin.Int64(), epochs.LteEnd.Int64(), name).
		Order("epoch desc").
		Find(&items).Error
	if err != nil {
		return
	}

	return
}

func (s SyncerDal) GetSyncTasksEpochs(ctx context.Context, names []string, epochs chain.LCRCRange) (items []*po.SyncTaskEpoch, err error) {

	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where("epoch >= ? and epoch <= ? and task in ?",
		epochs.GteBegin.Int64(), epochs.LteEnd.Int64(), names).
		Order("epoch desc").
		Find(&items).Error
	if err != nil {
		return
	}

	return
}
