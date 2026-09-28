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

// SaveSkippedEpoch 登记/更新一条跳过记录（含「区间跳」的 skipped_from / skipped_to / error_class）。
// 按 (syncer, epoch) 幂等 upsert，保留首次登记的 created_at/first_failed_at。
//
// 区间跳复用同一行：单高度跳过先写入 (syncer, epoch) 行，随后区间跳把该行升级为区间行
// （epoch = skipped_from，写入 skipped_to / error_class），因此不会出现重复行。
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
				"error_class":    item.ErrorClass,
				"skipped_from":   item.SkippedFrom,
				"skipped_to":     item.SkippedTo,
			}).Error
		}
	})
	return
}

// GetSkippedEpochs 查询指定同步器在 [begin, end] 区间内**有覆盖**的跳过记录（单高度行按 epoch 命中，
// 区间行按 [skipped_from, skipped_to] 与查询区间求交命中）。
func (s SyncerDal) GetSkippedEpochs(ctx context.Context, name string, epochs chain.LCRCRange) (items []*po.SyncSkippedEpoch, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	if err = epochs.Valid(); err != nil {
		return
	}
	// coalesce(skipped_to, epoch) 让单高度行（skipped_to 为 NULL）保持原有语义：
	// epoch ∈ [begin, end]；区间行则按区间右端参与求交（左端即 epoch <= end）。
	err = db.Where("syncer = ? and epoch <= ? and coalesce(skipped_to, epoch) >= ?",
		name, epochs.LteEnd.Int64(), epochs.GteBegin.Int64()).
		Order("epoch desc").
		Find(&items).Error
	if err != nil {
		return
	}
	return
}
