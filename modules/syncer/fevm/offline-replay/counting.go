package offlinereplay

import (
	"context"
	"sync/atomic"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件是「离线回放」工具链的计数基础：
//
//	① Counters  —— 写拦截计数器，各命令包的「只统计不落库」仓储包装都用它记账；
//	② CountingAgg —— 聚合器调用计数器（离线测量聚合器压力用）。

// WriteStat 单个派生表的写入统计快照。
type WriteStat struct {
	Table   string // 派生表名，如 fevm.evm_transfers
	Calls   int64  // 写方法被调用次数
	Rows    int64  // 被拦下的行数（真写模式下即会落库的行数）
	Deletes int64  // 删除方法被调用次数（回滚/清理路径）
}

// TotalRows 被拦下的行数合计
func (s WriteStat) TotalRows() int64 { return s.Rows }

// Counters 写拦截计数器（并发安全：同步器按高度并发跑任务）。
type Counters struct {
	table   string
	calls   atomic.Int64
	rows    atomic.Int64
	deletes atomic.Int64
}

// NewCounters 建一个针对某张派生表的计数器
func NewCounters(table string) *Counters { return &Counters{table: table} }

// Table 派生表名
func (c *Counters) Table() string { return c.table }

// CountWrite 记一次「写被拦下」（rows 为本次被拦下的行数）
func (c *Counters) CountWrite(rows int) {
	if c == nil {
		return
	}
	c.calls.Add(1)
	c.rows.Add(int64(rows))
}

// CountDelete 记一次「删除被拦下」
func (c *Counters) CountDelete() {
	if c == nil {
		return
	}
	c.deletes.Add(1)
}

// Snapshot 返回当前快照
func (c *Counters) Snapshot() WriteStat {
	if c == nil {
		return WriteStat{}
	}
	return WriteStat{Table: c.table, Calls: c.calls.Load(), Rows: c.rows.Load(), Deletes: c.deletes.Load()}
}

// SumWrites 汇总多张表的写入统计（报告用）
func SumWrites(stats []WriteStat) (calls, rows, deletes int64) {
	for _, s := range stats {
		calls += s.Calls
		rows += s.Rows
		deletes += s.Deletes
	}
	return
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
		panic("offlinereplay: NewCountingAgg 需要非 nil 的聚合器客户端")
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
