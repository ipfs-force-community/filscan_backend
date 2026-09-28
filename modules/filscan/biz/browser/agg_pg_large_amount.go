package browser

import (
	"context"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

// 本文件：把「大额转账列表」端点从「实时问聚合器（扇出 12 个冷库、实测 60 秒零字节挂住）」
// 切到「读 PG 预计算表 chain.large_transfers」（migration/35.large_transfers.sql）。
//
// 切换点是同一个装饰器模式（见 agg_pg_reward.go 的文件头：为什么在 API 侧包、为什么不进 wire provider）：
//
//	modules/filscan/biz/browser/biz.go 里 NewPgLargeAmountAgg 在原 agg 外面再包一层
//	⇒ acl 层（acl_block_chain.go 的 GetTransferLargeAmount）与 biz 层（Redis 缓存）**一行都不用改**。
//
// 行为不变性（默认零变化）：
//   - 开关关闭（或 db 为空）时 NewPgLargeAmountAgg 原样返回入参 agg，不构造装饰器、不构造 DAL、不发 SQL；
//   - 开关开启后只替换这一个端点的读路径，其余 100+ 个 agg 方法经 embedded interface 原样透传；
//   - PG 报错/超时/表缺失 ⇒ **回落聚合器**并打 Warn（宁慢不空，不让前端因为 PG 抖动 500）。
//
// 与聚合器的字段对齐（依据管线原文 transfer_message_for_large_amount.js 的 $project）：
//
//	Cid     ← 表 cid（管线里是 SignedCid||Cid 的解析结果）
//	RootCid ← 表 root_cid（管线里是 RootSignedCid||RootCid 的解析结果）
//	Epoch / From / To / Value / Method / Depth ← 同名列
//	SignedCid / ExitCode ← **保持零值**：管线的 $project 里没有这两个键
//	                        （SignedCid 被并进 Cid、ExitCode 由 $match=0 变成前提），
//	                        所以聚合器响应里它们本来就是 "" 与 0；
//	IsBlock              ← 只存在于聚合器的 TransferMessage，filscan 的 ActorMessages 无该字段，无需映射。
//
// 两处与线上的**已知差异**（不假装逐行一致）：
//
//  1. 同高度内行序：线上每个冷库只按 Epoch 排序、同高度内行序未定义；PG 侧定序为
//     (epoch desc, cid asc)。若分页窗口切在一个高度组中间，该页的行集合也可能与线上不同。
//  2. TotalCount 的来源：线上是各冷库区间行数求和（分片边界重叠会重复计 262 个 (epoch,cid)），
//     PG 是全表 count(*)。同为「不去重的行数」口径。
//
// 空页（offset 超出总量）必须返回 nil、nil：聚合器此时回的是 data:null，
// bindResult 把 nil 交给调用方（acl 的 `if transferList != nil` 不成立），
// 于是 biz 层缓存的是零值响应（total_count=0、列表 null）。跟着返回 nil 才能保持同一形态。

// PgLargeAmountOptions 大额转账端点的开关 + PG 读超时。
type PgLargeAmountOptions struct {
	LargeAmount bool
	Timeout     time.Duration // <= 0 表示不设超时
}

// Enabled 开关是否打开。关闭时调用方应直接返回原 agg（零开销、零行为变化）。
func (o PgLargeAmountOptions) Enabled() bool {
	return o.LargeAmount
}

// PgLargeAmountOptionsFromConfig 从配置读取开关。字段缺失 = 关闭（老配置行为不变）。
func PgLargeAmountOptionsFromConfig(conf *config.Config) PgLargeAmountOptions {
	opt := PgLargeAmountOptions{LargeAmount: conf.LargeAmountReadFromPg()}
	if ms := conf.PgReadTimeoutMs(); ms > 0 {
		opt.Timeout = time.Duration(ms) * time.Millisecond
	}
	return opt
}

// NewPgLargeAmountAgg 按配置装饰 agg；开关关闭（或 db 为空）时返回入参本身。
func NewPgLargeAmountAgg(agg londobell.Agg, db *gorm.DB, conf *config.Config) londobell.Agg {
	opt := PgLargeAmountOptionsFromConfig(conf)
	if !opt.Enabled() || db == nil || agg == nil {
		return agg
	}
	log.Infof("aggregator large amount endpoint switched to PG: large_amount=%v timeout=%s", opt.LargeAmount, opt.Timeout)
	return NewPgLargeAmountAggWithReader(agg, dal.NewLargeTransferDal(db), opt)
}

// NewPgLargeAmountAggWithReader 同上，但由调用方注入读实现（单测用）。
// 开关关闭时同样原样返回入参：装饰器即使是「空转」也不该存在，否则调用方会以为开了开关。
func NewPgLargeAmountAggWithReader(agg londobell.Agg, reader repository.LargeAmountTransfer, opt PgLargeAmountOptions) londobell.Agg {
	if !opt.Enabled() {
		return agg
	}
	return &pgLargeAmountAgg{Agg: agg, reader: reader, opt: opt}
}

var _ londobell.Agg = (*pgLargeAmountAgg)(nil)

// pgLargeAmountAgg 只覆盖 TransferLargeAmount 一个端点，其余方法经 embedded interface 原样透传。
type pgLargeAmountAgg struct {
	londobell.Agg
	reader repository.LargeAmountTransfer
	opt    PgLargeAmountOptions
}

// withTimeout 为单次 PG 读派生一个带超时的 ctx；Timeout<=0 时原样返回（不设超时）。
func (a *pgLargeAmountAgg) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.opt.Timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, a.opt.Timeout)
}

// TransferLargeAmount 聚合器端点 /aggregators/transfer_message_for_largeAmount → chain.large_transfers。
//
// index/limit 的语义：**index 是页码**（线上 skip = index*limit，见 dal.LargeTransferPageWindow），
// 折算后作为 SQL 的 offset/limit；index 与 limit 都为 0 时线上取全量，这里同样不设上限。
func (a *pgLargeAmountAgg) TransferLargeAmount(ctx context.Context, filters types.Filters) (*londobell.TransferLargeAmountList, error) {
	if !a.opt.LargeAmount {
		return a.Agg.TransferLargeAmount(ctx, filters)
	}

	offset, pageLimit := dal.LargeTransferPageWindow(filters.Index, filters.Limit)

	listCtx, cancelList := a.withTimeout(ctx)
	rows, err := a.reader.LargeTransfersPage(listCtx, offset, pageLimit)
	cancelList()
	if err != nil {
		log.Warnf("read large amount transfers from pg failed (index=%d limit=%d offset=%d): %s; fallback to aggregator",
			filters.Index, filters.Limit, offset, err)
		return a.Agg.TransferLargeAmount(ctx, filters)
	}

	// 空页：与聚合器 data:null 的形态一致（调用方按 nil 处理，TotalCount 同样不返回）。
	if len(rows) == 0 {
		return nil, nil
	}

	// 每条 SQL 各自一个超时（列表与计数是两次独立查询）。
	countCtx, cancelCount := a.withTimeout(ctx)
	total, err := a.reader.CountLargeTransfers(countCtx)
	cancelCount()
	if err != nil {
		// TotalCount 缺失会让前端页码错乱 ⇒ 不做「列表用 PG、计数缺省 0」的降级，整条回落到聚合器。
		log.Warnf("count large amount transfers from pg failed (index=%d limit=%d): %s; fallback to aggregator",
			filters.Index, filters.Limit, err)
		return a.Agg.TransferLargeAmount(ctx, filters)
	}

	return &londobell.TransferLargeAmountList{
		TotalCount:          total,
		TransferLargeAmount: largeTransferRowsToActorMessages(rows),
	}, nil
}

// largeTransferRowsToActorMessages 逐行映射成聚合器返回结构。
// SignedCid / ExitCode 显式留零值：聚合器管线的 $project 里没有这两个键（见文件头）。
func largeTransferRowsToActorMessages(rows []*bo.LargeTransferRow) []*londobell.ActorMessages {
	out := make([]*londobell.ActorMessages, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, &londobell.ActorMessages{
			Cid:     row.Cid,
			RootCid: row.RootCid,
			Epoch:   row.Epoch,
			From:    chain.SmartAddress(row.FromAddr),
			To:      chain.SmartAddress(row.ToAddr),
			Value:   row.Value,
			Method:  row.Method,
			Depth:   row.Depth,
		})
	}
	return out
}
