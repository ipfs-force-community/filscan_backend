package rewardparity

import (
	"fmt"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件：大额转账列表端点（/aggregators/transfer_message_for_largeAmount）的两路径比对。
//
// 与三个统计端点的区别（决定了比对策略不同）：
//
//  1. 该端点**只吃 index/limit**（没有高度区间）⇒ 比对的是「同一页」，不是「同一区间」；
//  2. TotalCount 是全表行数（线上是各冷库区间行数求和），且**必须含重复行**（口径的一部分）；
//  3. 线上**同高度内行序未定义**（每个冷库只 $sort:{Epoch:-1}，库间合并顺序也未定义）
//     ⇒ 逐位置严格比对必然失败，故：
//     - 逐行按**多重集**比对（键 = epoch|cid|root_cid，带重数），缺失/多出才是真错；
//     - 顺序差异单独统计为 OrderDiffCount（默认不判失败，-strict-order 可升级）。
//
// PG 侧自己的定序（order by epoch desc, cid asc）必须成立：epoch 一旦出现递增就是 PG 侧 bug，
// 记 SortViolations（永远致命）—— 这与「同高度内行序差异」是两回事，不要混为一谈。

// LargeAmountParams 大额转账端点的一次比对范围。
type LargeAmountParams struct {
	Index     int64 // 请求的页码（线上 skip = index*limit）
	Limit     int64 // 请求的每页条数
	Offset    int64 // PG 侧实际 offset（= index*limit，index=limit=0 时为 0）
	PageLimit int64 // PG 侧实际 limit（index=limit=0 时为 MaxInt64：线上此时取全量）
	// PgOnly true = 只跑了 PG 侧（聚合器不可用时的自检）：不做两侧比对，只做 PG 侧内部一致性检查。
	PgOnly bool
}

// CompareLargeAmount 比对同一页的两条路径结果。
//
// agg 为 nil = 聚合器回了 data:null（空页，或 PgOnly 模式下没取）；此时：
//
//	非 PgOnly：两侧行数都按 0 处理，TotalCount 仍要比（聚合器空页时 total 也是 0）。
//	PgOnly   ：跳过两侧比对，只输出 PG 侧统计与定序检查。
func CompareLargeAmount(p LargeAmountParams, agg *londobell.TransferLargeAmountList, pgRows []*bo.LargeTransferRow, pgTotal int64, maxEx MaxExamples) Result {
	r := Result{Endpoint: EndpointLargeAmount, PgOnlyMode: p.PgOnly, PgTotal: pgTotal}
	c := newRecorder(maxEx.limit())

	// ---- PG 侧自检：定序必须 epoch 非递增（order by epoch desc, cid asc） ----
	for i := 1; i < len(pgRows); i++ {
		prev, cur := pgRows[i-1], pgRows[i]
		if prev == nil || cur == nil {
			continue
		}
		if cur.Epoch > prev.Epoch {
			r.SortViolations++
			if len(r.SortSamples) < c.limit {
				r.SortSamples = append(r.SortSamples, fmt.Sprintf("位置 %d: epoch %d 在 %d 之后（应为非递增）", i, cur.Epoch, prev.Epoch))
			}
		}
	}

	var aggRows []*londobell.ActorMessages
	if agg != nil {
		aggRows = agg.TransferLargeAmount
		r.AggTotal = agg.TotalCount
	}
	r.AggRows, r.PgRows = len(aggRows), len(pgRows)
	r.PgTotal = pgTotal

	if !p.PgOnly {
		r.TotalDiff = r.AggTotal != r.PgTotal
		r.compareLargeAmountRows(c, aggRows, pgRows)
	}

	c.finish(&r)
	return r
}

// compareLargeAmountRows 逐行比对（多重集：键相同、重数相同、字段逐个相同）。
func (r *Result) compareLargeAmountRows(c *recorder, aggRows []*londobell.ActorMessages, pgRows []*bo.LargeTransferRow) {
	aggByKey, aggKeyOrder := groupAggRows(aggRows)
	pgByKey, pgKeyOrder := groupPgRows(pgRows)

	seen := make(map[string]bool, len(aggKeyOrder))
	for _, key := range aggKeyOrder {
		aggList, pgList := aggByKey[key], pgByKey[key]
		seen[key] = true
		if !r.PgOnlyMode {
			n := min(len(aggList), len(pgList))
			for i := 0; i < n; i++ {
				compareLargeAmountRow(c, key, aggList[i], pgList[i])
			}
			// 重数不同：跨库边界重复行缺失（PG 少）或多出（PG 多）都记差异，且**逐行**计数。
			for i := 0; i < len(aggList)-n; i++ {
				c.recordAggOnly(key)
			}
			for i := 0; i < len(pgList)-n; i++ {
				c.recordPgOnly(key)
			}
		}
	}
	for _, key := range pgKeyOrder {
		if seen[key] {
			continue
		}
		for range pgByKey[key] {
			c.recordPgOnly(key)
		}
	}

	// 同高度内行序差异：只在两侧**行集合与重数完全一致**时才谈顺序（否则差异已被上面的点位差异覆盖）。
	if r.PgOnlyMode || c.points.aggOnlyCount > 0 || c.points.pgOnlyCount > 0 {
		return
	}
	n := min(len(aggRows), len(pgRows))
	for i := 0; i < n; i++ {
		aggKey := aggLargeAmountKey(aggRows[i])
		pgKey := pgLargeAmountKey(pgRows[i])
		if aggKey == pgKey {
			continue
		}
		r.OrderDiffCount++
		if len(r.OrderDiffs) < c.limit {
			r.OrderDiffs = append(r.OrderDiffs, fmt.Sprintf("位置 %d: 聚合器=%s PG=%s", i, aggKey, pgKey))
		}
	}
}

// largeAmountKey 行身份键：epoch|cid|root_cid。
//
// 为什么键里必须带 root_cid：同一 (epoch,cid) 可能有多行 —— 同高度竞争区块各留一行 trace、
// 且跨库边界重复的 262 个 (epoch,cid) 出现在相邻两库（内容逐字节相同）。这些行线上都会返回，
// 所以键相同、重数也必须相同才算一致。
func largeAmountKey(epoch int64, cid, rootCid string) string {
	return fmt.Sprintf("%d|%s|%s", epoch, cid, rootCid)
}

func aggLargeAmountKey(m *londobell.ActorMessages) string {
	if m == nil {
		return ""
	}
	return largeAmountKey(m.Epoch, m.Cid, m.RootCid)
}

func pgLargeAmountKey(row *bo.LargeTransferRow) string {
	if row == nil {
		return ""
	}
	return largeAmountKey(row.Epoch, row.Cid, row.RootCid)
}

func groupAggRows(rows []*londobell.ActorMessages) (map[string][]*londobell.ActorMessages, []string) {
	byKey := make(map[string][]*londobell.ActorMessages, len(rows))
	var order []string
	for _, row := range rows {
		key := aggLargeAmountKey(row)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], row)
	}
	return byKey, order
}

func groupPgRows(rows []*bo.LargeTransferRow) (map[string][]*bo.LargeTransferRow, []string) {
	byKey := make(map[string][]*bo.LargeTransferRow, len(rows))
	var order []string
	for _, row := range rows {
		key := pgLargeAmountKey(row)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], row)
	}
	return byKey, order
}

func compareLargeAmountRow(c *recorder, key string, agg *londobell.ActorMessages, pg *bo.LargeTransferRow) {
	c.recordString(key, "Cid", agg.Cid, pg.Cid)
	c.recordString(key, "RootCid", agg.RootCid, pg.RootCid)
	c.recordInt64(key, "Epoch", agg.Epoch, pg.Epoch)
	c.recordAddress(key, "From", agg.From, pg.FromAddr)
	c.recordAddress(key, "To", agg.To, pg.ToAddr)
	c.recordDecimal(key, "Value", agg.Value, pg.Value)
	c.recordString(key, "Method", agg.Method, pg.Method)
	c.recordInt64(key, "Depth", agg.Depth, pg.Depth)
	// 下面两个字段在聚合器管线里根本没有（SignedCid 被并进 Cid、ExitCode 由 $match=0 变成前提）
	// ⇒ 两侧都必须为零值。显式比一比，防止将来谁「顺手」把 SignedCid 填上造成静默差异。
	c.recordString(key, "SignedCid", agg.SignedCid, "")
	c.recordInt64(key, "ExitCode", agg.ExitCode, 0)
}

// recordString 文本字段比对（完全一致才通过）。
func (c *recorder) recordString(key, field, agg, pg string) {
	c.record(key, field, agg, pg, agg == pg, agg != pg)
}

// recordAddress 地址字段比对：语义（不带前缀归一后）不同 = 真错；只有原文形态不同 = format-only 差异。
//
// 归一理由与 normalizeMiner 一致（见该函数注释）：filscan 输出给前端的地址是
// SmartAddress.Address()（缺前缀会补 f/t），所以带不带前缀不影响接口输出，只影响原始字段文本。
func (c *recorder) recordAddress(key, field string, agg chain.SmartAddress, pg string) {
	aggRaw, pgRaw := string(agg), pg
	same := agg.Address() == chain.SmartAddress(pg).Address()
	if same && aggRaw == pgRaw {
		return
	}
	c.record(key, field, aggRaw, pgRaw, same, true)
}
