package repository

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// SyncerSkipLedger 同步器「数据级错误跳过台账」读写接口。
//
// 单独成接口（而不是并入 SyncerRepo）：同步器的跳过逻辑只依赖这两个方法，
// 便于单测注入假实现，也避免改动既有 SyncerRepo 的所有实现方。
// 实现见 dal.SyncerDal。
type SyncerSkipLedger interface {
	// SaveSkippedEpoch 登记/更新一条跳过记录（按 syncer+epoch 幂等 upsert：
	// 首次写入 created_at/first_failed_at，重复写入更新 error_message/failures/last_failed_at）
	SaveSkippedEpoch(ctx context.Context, item *po.SyncSkippedEpoch) (err error)
	// GetSkippedEpochs 查询指定同步器在 [begin, end] 区间内被跳过的高度
	GetSkippedEpochs(ctx context.Context, name string, epochs chain.LCRCRange) (items []*po.SyncSkippedEpoch, err error)
}
