package dal

import (
	"context"

	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

var _ repository.SyncerSkipLedger = (*SyncerDal)(nil)

// SaveSkippedEpoch 登记/更新一条「数据级错误跳过」记录。
// 按 (syncer, epoch) 幂等 upsert，保留首次登记的 created_at/first_failed_at。
func (s SyncerDal) SaveSkippedEpoch(ctx context.Context, item *po.SyncSkippedEpoch) (err error) {
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		existing := new(po.SyncSkippedEpoch)
		e := tx.Where("syncer=? and epoch=?", item.Syncer, item.Epoch).First(existing).Error
		switch {
		case errors.Is(e, gorm.ErrRecordNotFound):
			return tx.Create(item).Error
		case e != nil:
			return e
		default:
			return tx.Model(existing).Updates(map[string]interface{}{
				"error_message":  item.ErrorMessage,
				"failures":       item.Failures,
				"last_failed_at": item.LastFailedAt,
			}).Error
		}
	})
	return
}

// GetSkippedEpochs 查询指定同步器在 [begin, end] 区间内被跳过的高度
func (s SyncerDal) GetSkippedEpochs(ctx context.Context, name string, epochs chain.LCRCRange) (items []*po.SyncSkippedEpoch, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	if err = epochs.Valid(); err != nil {
		return
	}
	err = db.Where("syncer = ? and epoch >= ? and epoch <= ?",
		name, epochs.GteBegin.Int64(), epochs.LteEnd.Int64()).
		Order("epoch desc").
		Find(&items).Error
	if err != nil {
		return
	}
	return
}
