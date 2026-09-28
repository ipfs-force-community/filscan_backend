package evmtransfercmd

import (
	"context"
	"sync/atomic"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件实现「只统计不落库」所需的两个包装层：
//
//	① NoWriteEvmTransferRepo —— 包住派生表仓储，把 fevm.evm_transfers / fevm.evm_transfer_stats
//	   的写入（含删除）全部拦下，只累计「调用次数 / 行数」；读操作由嵌入接口原样透传。
//	② CountingAgg —— 包住聚合器客户端，累计各方法的调用次数（离线测量聚合器压力用）。

// NoWriteStats 只统计不落库模式的累计快照。
type NoWriteStats struct {
	TransferCalls int64 // SaveEvmTransfers 被调用次数
	TransferRows  int64 // 被拦下的 fevm.evm_transfers 行数
	StatCalls     int64 // SaveEvmTransferStats 被调用次数
	StatRows      int64 // 被拦下的 fevm.evm_transfer_stats 行数
	DeleteCalls   int64 // DeleteEvmTransfers / DeleteEvmTransferStats 被调用次数（回滚路径）
}

// TotalRows 被拦下的写入总行数（若真写，这些行会落到派生表）
func (s NoWriteStats) TotalRows() int64 {
	return s.TransferRows + s.StatRows
}

// NoWriteEvmTransferRepo 是 repository.EvmTransferRepo 的「只统计不落库」包装：
// 写方法（Save/Delete 派生表）一律不调用底层仓储，只计数；其余方法（读）由嵌入接口原样透传。
//
// 安全性质：本类型的写方法体内**不存在**对底层仓储写方法的调用，因此即使上层 task 逻辑变化，
// 也不可能经由本类型写入 fevm.evm_transfers / fevm.evm_transfer_stats。
type NoWriteEvmTransferRepo struct {
	// 嵌入底层仓储：读方法（GetEvmTransferStats 等）原样透传，只有写方法被本类型覆盖拦下
	repository.EvmTransferRepo

	transferCalls atomic.Int64
	transferRows  atomic.Int64
	statCalls     atomic.Int64
	statRows      atomic.Int64
	deleteCalls   atomic.Int64
}

var _ repository.EvmTransferRepo = (*NoWriteEvmTransferRepo)(nil)

// NewNoWriteEvmTransferRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）。
func NewNoWriteEvmTransferRepo(inner repository.EvmTransferRepo) *NoWriteEvmTransferRepo {
	if inner == nil {
		panic("evmtransfercmd: NewNoWriteEvmTransferRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteEvmTransferRepo{EvmTransferRepo: inner}
}

// Inner 返回底层仓储（读操作透传目标）
func (r *NoWriteEvmTransferRepo) Inner() repository.EvmTransferRepo { return r.EvmTransferRepo }

func (r *NoWriteEvmTransferRepo) SaveEvmTransfers(_ context.Context, infos []*po.EvmTransfer) error {
	r.transferCalls.Add(1)
	r.transferRows.Add(int64(len(infos)))
	return nil
}

func (r *NoWriteEvmTransferRepo) SaveEvmTransferStats(_ context.Context, infos []*po.EvmTransferStat) error {
	r.statCalls.Add(1)
	r.statRows.Add(int64(len(infos)))
	return nil
}

func (r *NoWriteEvmTransferRepo) DeleteEvmTransfers(_ context.Context, _ chain.Epoch) error {
	r.deleteCalls.Add(1)
	return nil
}

func (r *NoWriteEvmTransferRepo) DeleteEvmTransferStats(_ context.Context, _ chain.Epoch) error {
	r.deleteCalls.Add(1)
	return nil
}

// Stats 返回累计快照
func (r *NoWriteEvmTransferRepo) Stats() NoWriteStats {
	return NoWriteStats{
		TransferCalls: r.transferCalls.Load(),
		TransferRows:  r.transferRows.Load(),
		StatCalls:     r.statCalls.Load(),
		StatRows:      r.statRows.Load(),
		DeleteCalls:   r.deleteCalls.Load(),
	}
}

// AggStats 聚合器各方法的调用次数快照。
type AggStats struct {
	Traces        int64 // Traces（每个高度 1 次，由 SetTracesBuilder 调用）
	Tipsets       int64 // Tipset（每个高度 2 次：判空一次 + 取 tipset 身份一次）
	ParentTipsets int64 // ParentTipset（每个高度 1 次）
	LatestTipsets int64 // LatestTipset（每轮 run() 1 次）
}

// Total 聚合器调用总次数
func (s AggStats) Total() int64 {
	return s.Traces + s.Tipsets + s.ParentTipsets + s.LatestTipsets
}

// CountingAgg 是 londobell.Agg 的计数包装：只累计离线回放实际用到的 4 个方法的调用次数，
// 其余方法由嵌入接口透传。
type CountingAgg struct {
	londobell.Agg

	traces        atomic.Int64
	tipsets       atomic.Int64
	parentTipsets atomic.Int64
	latestTipsets atomic.Int64
}

var _ londobell.Agg = (*CountingAgg)(nil)

// NewCountingAgg 包装聚合器客户端（inner 必须非 nil）。
func NewCountingAgg(inner londobell.Agg) *CountingAgg {
	if inner == nil {
		panic("evmtransfercmd: NewCountingAgg 需要非 nil 的聚合器客户端")
	}
	return &CountingAgg{Agg: inner}
}

// Inner 返回底层聚合器客户端
func (a *CountingAgg) Inner() londobell.Agg { return a.Agg }

func (a *CountingAgg) Traces(ctx context.Context, start, end chain.Epoch) ([]*londobell.TraceMessage, error) {
	a.traces.Add(1)
	return a.Agg.Traces(ctx, start, end)
}

func (a *CountingAgg) Tipset(ctx context.Context, epoch chain.Epoch) ([]*londobell.Tipset, error) {
	a.tipsets.Add(1)
	return a.Agg.Tipset(ctx, epoch)
}

func (a *CountingAgg) ParentTipset(ctx context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	a.parentTipsets.Add(1)
	return a.Agg.ParentTipset(ctx, start)
}

func (a *CountingAgg) LatestTipset(ctx context.Context) ([]*londobell.Tipset, error) {
	a.latestTipsets.Add(1)
	return a.Agg.LatestTipset(ctx)
}

// Stats 返回聚合器调用次数快照
func (a *CountingAgg) Stats() AggStats {
	return AggStats{
		Traces:        a.traces.Load(),
		Tipsets:       a.tipsets.Load(),
		ParentTipsets: a.parentTipsets.Load(),
		LatestTipsets: a.latestTipsets.Load(),
	}
}
