package evmtransfercmd

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// NoWriteEvmTransferRepo 是 repository.EvmTransferRepo 的「只统计不落库」包装：
// 写方法（Save/Delete 派生表）一律**不调用**底层仓储，只往计数器记账；读方法由嵌入接口原样透传。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使上层 task 逻辑变化，
// 也不可能经由本类型写入 fevm.evm_transfers / fevm.evm_transfer_stats。
type NoWriteEvmTransferRepo struct {
	// 嵌入底层仓储：读方法（GetEvmTransferStats 等）原样透传，只有写方法被本类型覆盖拦下
	repository.EvmTransferRepo

	transfers *offlinereplay.Counters // fevm.evm_transfers
	stats     *offlinereplay.Counters // fevm.evm_transfer_stats
}

var _ repository.EvmTransferRepo = (*NoWriteEvmTransferRepo)(nil)

// 派生表名（与 po.EvmTransfer / po.EvmTransferStat 的 TableName 一致）
const (
	TableEvmTransfers     = "fevm.evm_transfers"
	TableEvmTransferStats = "fevm.evm_transfer_stats"
)

// NewNoWriteEvmTransferRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）。
func NewNoWriteEvmTransferRepo(inner repository.EvmTransferRepo) *NoWriteEvmTransferRepo {
	if inner == nil {
		panic("evmtransfercmd: NewNoWriteEvmTransferRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteEvmTransferRepo{
		EvmTransferRepo: inner,
		transfers:       offlinereplay.NewCounters(TableEvmTransfers),
		stats:           offlinereplay.NewCounters(TableEvmTransferStats),
	}
}

// Inner 返回底层仓储（读操作透传目标）
func (r *NoWriteEvmTransferRepo) Inner() repository.EvmTransferRepo { return r.EvmTransferRepo }

// SaveEvmTransfers 拦下 fevm.evm_transfers 写入，只计数（行数 = 本次本会写入的行数）
func (r *NoWriteEvmTransferRepo) SaveEvmTransfers(_ context.Context, infos []*po.EvmTransfer) error {
	r.transfers.CountWrite(len(infos))
	return nil
}

// SaveEvmTransferStats 拦下 fevm.evm_transfer_stats 写入，只计数
func (r *NoWriteEvmTransferRepo) SaveEvmTransferStats(_ context.Context, infos []*po.EvmTransferStat) error {
	r.stats.CountWrite(len(infos))
	return nil
}

// DeleteEvmTransfers 拦下删除（回滚路径），只计数
func (r *NoWriteEvmTransferRepo) DeleteEvmTransfers(_ context.Context, _ chain.Epoch) error {
	r.transfers.CountDelete()
	return nil
}

// DeleteEvmTransferStats 拦下删除（回滚路径），只计数
func (r *NoWriteEvmTransferRepo) DeleteEvmTransferStats(_ context.Context, _ chain.Epoch) error {
	r.stats.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteEvmTransferRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{r.transfers.Snapshot(), r.stats.Snapshot()}
}

// TransferStats 兼容旧口径的计数快照（单测断言用）
func (r *NoWriteEvmTransferRepo) TransferStats() (calls, rows, deletes int64) {
	s := r.transfers.Snapshot()
	return s.Calls, s.Rows, s.Deletes
}

// StatStats 兼容旧口径的计数快照（单测断言用）
func (r *NoWriteEvmTransferRepo) StatStats() (calls, rows, deletes int64) {
	s := r.stats.Snapshot()
	return s.Calls, s.Rows, s.Deletes
}
