// Package rewardparity 比对「聚合器（londobell 冷库扇出）」与「PG 派生表」两条读路径的逐字段结果。
//
// 用途：三个统计端点（miner_blockreward / miners_blockreward / wincount）切到 PG 之前的
// **通关条件**。给定 (miner / 高度区间)，两条路径各取一次数，逐字段、逐点位打印差异：
//
//   - 点位集合：分别在两条路径上出现、对方没有的 key（覆盖缺口 / 区间口径 off-by-one）
//   - 字段值：数值不等（真错）与「数值相等但文本形态不同」（金额格式风险，前端展示会变）
//   - 无来源字段：聚合器返回但 PG 没有对应列（wincount 的 TotalGasReward）
//
// 计数（*Count 字段）是**全量**的，Diffs/AggOnly/PgOnly 只是**最多 N 条样例**：
// 报告里的数字可以直接当门禁用，样例只用于定位。
//
// 本包只做「比对」：不发请求也不连库；取数由调用方（cmd/agg-parity）负责。
package rewardparity

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 端点名：与 londobell 的 HTTP path 同名，日志/报告里可直接对照。
const (
	EndpointMinerBlockReward  = "miner_blockreward"
	EndpointMinersBlockReward = "miners_blockreward"
	EndpointWinCount          = "wincount"
	// EndpointLargeAmount 大额转账列表端点 /aggregators/transfer_message_for_largeAmount。
	// 与上面三个不同：它只吃 index/limit（没有高度区间），TotalCount 是全表行数。
	EndpointLargeAmount = "large_amount"
)

// AllEndpoints 默认比对的端点：三个统计端点。
// 大额转账需显式 `-endpoints large_amount`：它不按高度区间取数（-start/-end 对它无意义），
// 且线上该端点极慢（实测 60 秒零字节挂住），不适合放进默认集合。
func AllEndpoints() []string {
	return []string{EndpointMinerBlockReward, EndpointMinersBlockReward, EndpointWinCount}
}

// ValidEndpoints 可选择的全部端点（含大额转账），供工具做参数校验与帮助文本。
func ValidEndpoints() []string {
	return append(AllEndpoints(), EndpointLargeAmount)
}

// NeedsEpochRange 该端点是否按高度区间取数（只有三个统计端点需要 -start/-end）。
func NeedsEpochRange(endpoint string) bool {
	return endpoint != EndpointLargeAmount
}

// FieldDiff 一条字段级差异（样例）。
type FieldDiff struct {
	Key     string // 点位标识：epoch 或 epoch|miner 或 miner
	Field   string // 聚合器返回值的字段名（TotalBlockReward / BlockCount / TotalWinCount / TotalGasReward）
	Agg     string // 聚合器侧文本
	Pg      string // PG 侧文本
	ValueEq bool   // 数值是否相等：false ⇒ 真错；true ⇒ 仅文本形态不同（金额格式风险）
}

// 点位标识类型：missing* 是「只在一边出现」的点位。
type pointSet struct {
	aggOnly      []string
	pgOnly       []string
	aggOnlyCount int
	pgOnlyCount  int
}

// Params 一次比对的范围（区间语义左闭右开，与聚合器管线一致）。
type Params struct {
	Miner chain.SmartAddress // 空 = 不按矿工过滤
	Start chain.Epoch
	End   chain.Epoch
}

// Result 单个端点的比对结果。
type Result struct {
	Endpoint string
	Params   Params

	AggRows int
	PgRows  int

	points pointSet

	// Diffs 字段差异样例（最多 MaxExamples 条）；DiffCount/ValueDiffCount/FormatOnlyCount 是全量计数。
	Diffs          []FieldDiff
	DiffCount      int
	ValueDiffCount int
	FormatOnlyDiff int

	// Unresolved 聚合器返回、但 PG 侧没有数据来源的字段（数据模型差异，需人工决策，不是代码 bug）。
	Unresolved []string

	// 汇总：两边总量对照，用于一眼判读（数值要逐字段比对，但也别丢了总量这层放大镜）。
	AggSumReward decimal.Decimal
	PgSumReward  decimal.Decimal
	AggSumBlock  int64
	PgSumBlock   int64
	AggSumWin    int64
	PgSumWin     int64
	AggSumGas    decimal.Decimal

	AggLatency time.Duration
	PgLatency  time.Duration

	AggErr error
	PgErr  error

	// ===== 以下字段只由大额转账端点（large_amount）填写；三个统计端点保持 0/false =====

	// AggTotal / PgTotal 两侧 TotalCount 的对照值（聚合器是各冷库区间行数之和，PG 是全表 count(*)）。
	AggTotal int64
	PgTotal  int64
	// TotalDiff 两侧 TotalCount 不等。默认致命；工具可用 -lenient-total 降级为提示
	// （聚合器的计数来自另一条管线且带缓存，陈旧时差额不代表 PG 有错）。
	TotalDiff bool
	// OrderDiffCount 同高度内行序差异条数。这是**已知差异**（线上同高度内行序未定义，
	// PG 侧按 (epoch desc, cid asc) 定序），默认不判失败；工具可用 -strict-order 升级为失败。
	OrderDiffCount int
	OrderDiffs     []string
	// SortViolations PG 侧 epoch 倒序被破坏的条数（PG 自己的 order by 失效/表被写坏）⇒ 永远致命。
	SortViolations int
	SortSamples    []string
	// PgOnlyMode true = 只跑了 PG 侧（聚合器不可用时的自检）：未与聚合器比对，
	// AggRows/AggTotal/点位差异均无意义。
	PgOnlyMode bool
}

// AggOnly 只在聚合器侧出现的点位样例。
func (r Result) AggOnly() []string { return r.points.aggOnly }

// PgOnly 只在 PG 侧出现的点位样例。
func (r Result) PgOnly() []string { return r.points.pgOnly }

// AggOnlyCount 只在聚合器侧出现的点位数（覆盖缺口：PG 少了数据）。
func (r Result) AggOnlyCount() int { return r.points.aggOnlyCount }

// PgOnlyCount 只在 PG 侧出现的点位数（PG 多了数据：重复行/口径差异）。
func (r Result) PgOnlyCount() int { return r.points.pgOnlyCount }

// Fatal 与数值/点位/取数有关的问题数——与 strictFormat 无关，一律算失败。
func (r Result) Fatal() int {
	if r.AggErr != nil || r.PgErr != nil {
		return 1
	}
	return r.ValueDiffCount + r.AggOnlyCount() + r.PgOnlyCount()
}

// Pass strictFormat 为真时，金额文本形态差异也算失败（「格式必须完全一致」）。
func (r Result) Pass(strictFormat bool) bool {
	return r.PassDetailed(strictFormat, true, false)
}

// PassDetailed 三个策略显式传入（大额转账端点需要一个「同高度内行序不算失败」的默认）：
//
//	strictFormat 金额/地址的**文本形态**差异是否算失败（默认 true）
//	strictTotal  两侧 TotalCount 不等是否算失败（大额转账端点默认 true，可用 -lenient-total 关掉）
//	strictOrder  同高度内行序差异是否算失败（默认 false：线上本就没有该顺序）
//
// 与数值/点位/取数有关的差异（ValueDiffCount / 点位缺失 / 排序违规 / 取数错误）永远算失败。
func (r Result) PassDetailed(strictFormat, strictTotal, strictOrder bool) bool {
	if r.AggErr != nil || r.PgErr != nil {
		return false
	}
	if r.ValueDiffCount > 0 || r.AggOnlyCount() > 0 || r.PgOnlyCount() > 0 || r.SortViolations > 0 {
		return false
	}
	if strictTotal && r.TotalDiff {
		return false
	}
	if strictFormat && r.FormatOnlyDiff > 0 {
		return false
	}
	if strictOrder && r.OrderDiffCount > 0 {
		return false
	}
	return true
}

// MaxExamples 每个差异桶最多保留多少条样例（<=0 取 10）。
type MaxExamples int

func (m MaxExamples) limit() int {
	if m <= 0 {
		return 10
	}
	return int(m)
}

// recorder 收集差异：计数全量、样例截断。
type recorder struct {
	limit int

	diffs          []FieldDiff
	diffCount      int
	valueDiffCount int
	formatOnlyDiff int

	points pointSet
}

func newRecorder(limit int) *recorder {
	return &recorder{limit: limit}
}

// record 记录一次字段比对；一致（数值 + 文本）则什么都不做。
func (c *recorder) record(key, field string, agg, pg string, valueEq, mismatched bool) {
	if !mismatched {
		return
	}
	c.diffCount++
	if valueEq {
		c.formatOnlyDiff++
	} else {
		c.valueDiffCount++
	}
	if len(c.diffs) < c.limit {
		c.diffs = append(c.diffs, FieldDiff{Key: key, Field: field, Agg: agg, Pg: pg, ValueEq: valueEq})
	}
}

// recordDecimal 金额字段比对：数值相等 + 文本一致才算通过。
//
// 为什么文本也要比：接口返回的是 shopspring decimal 的 String()（MarshalJSON → 带引号字符串），
// 前端按文本展示；PG numeric 与聚合器 decimal128 的来源不同，任何形态差异都是展示风险。
// 反过来，String() 会砍掉尾随零 ⇒ PG numeric 的标度差异（1.0 vs 1）不会误报，落进来的都是真形态差异。
func (c *recorder) recordDecimal(key, field string, agg, pg decimal.Decimal) {
	c.record(key, field, agg.String(), pg.String(), agg.Equal(pg), !(agg.Equal(pg) && agg.String() == pg.String()))
}

// recordInt64 整数字段比对。
func (c *recorder) recordInt64(key, field string, agg, pg int64) {
	c.record(key, field, fmt.Sprintf("%d", agg), fmt.Sprintf("%d", pg), agg == pg, agg != pg)
}

func (c *recorder) recordAggOnly(key string) {
	c.points.aggOnlyCount++
	if len(c.points.aggOnly) < c.limit {
		c.points.aggOnly = append(c.points.aggOnly, key)
	}
}

func (c *recorder) recordPgOnly(key string) {
	c.points.pgOnlyCount++
	if len(c.points.pgOnly) < c.limit {
		c.points.pgOnly = append(c.points.pgOnly, key)
	}
}

func (c *recorder) finish(r *Result) {
	sort.Strings(c.points.aggOnly)
	sort.Strings(c.points.pgOnly)
	r.points = c.points
	r.Diffs = c.diffs
	r.DiffCount = c.diffCount
	r.ValueDiffCount = c.valueDiffCount
	r.FormatOnlyDiff = c.formatOnlyDiff
}

// CompareMinerBlockReward 比对 miner_blockreward：聚合器按 epoch 分组（Id=epoch）。
func CompareMinerBlockReward(p Params, agg []*londobell.MinerBlockReward, pg []*bo.MinerEpochReward, maxEx MaxExamples) Result {
	r := Result{Endpoint: EndpointMinerBlockReward, Params: p, AggRows: len(agg), PgRows: len(pg)}
	c := newRecorder(maxEx.limit())

	pgByEpoch := make(map[int64]*bo.MinerEpochReward, len(pg))
	for _, row := range pg {
		pgByEpoch[row.Epoch] = row
		r.PgSumReward = r.PgSumReward.Add(row.Reward)
		r.PgSumBlock += row.BlockCount
	}

	seen := make(map[int64]bool, len(agg))
	for _, item := range agg {
		if item == nil {
			continue
		}
		r.AggSumReward = r.AggSumReward.Add(item.TotalBlockReward)
		r.AggSumBlock += item.BlockCount

		key := fmt.Sprintf("%d", item.Id)
		seen[item.Id] = true
		row, ok := pgByEpoch[item.Id]
		if !ok {
			c.recordAggOnly(key)
			continue
		}
		c.recordDecimal(key, "TotalBlockReward", item.TotalBlockReward, row.Reward)
		c.recordInt64(key, "BlockCount", item.BlockCount, row.BlockCount)
	}
	for _, row := range pg {
		if !seen[row.Epoch] {
			c.recordPgOnly(fmt.Sprintf("%d", row.Epoch))
		}
	}
	c.finish(&r)
	return r
}

// CompareMinersBlockReward 比对 miners_blockreward：聚合器按 (epoch, miner) 分组。
func CompareMinersBlockReward(p Params, agg []*londobell.MinersBlockReward, pg []*bo.MinerEpochReward, maxEx MaxExamples) Result {
	r := Result{Endpoint: EndpointMinersBlockReward, Params: p, AggRows: len(agg), PgRows: len(pg)}
	c := newRecorder(maxEx.limit())

	pgByKey := make(map[string]*bo.MinerEpochReward, len(pg))
	for _, row := range pg {
		pgByKey[minerEpochKey(row.Epoch, row.Miner)] = row
		r.PgSumReward = r.PgSumReward.Add(row.Reward)
		r.PgSumBlock += row.BlockCount
	}

	seen := make(map[string]bool, len(agg))
	for _, item := range agg {
		if item == nil {
			continue
		}
		r.AggSumReward = r.AggSumReward.Add(item.TotalBlockReward)
		r.AggSumBlock += item.BlockCount

		key := minerEpochKey(item.Id.Epoch, item.Id.Miner)
		seen[key] = true
		row, ok := pgByKey[key]
		if !ok {
			c.recordAggOnly(key)
			continue
		}
		c.recordDecimal(key, "TotalBlockReward", item.TotalBlockReward, row.Reward)
		c.recordInt64(key, "BlockCount", item.BlockCount, row.BlockCount)
	}
	for _, row := range pg {
		k := minerEpochKey(row.Epoch, row.Miner)
		if !seen[k] {
			c.recordPgOnly(k)
		}
	}
	c.finish(&r)
	return r
}

// CompareWinCount 比对 wincount：聚合器按 miner 分组（Id=miner），PG 侧无 gas reward 列。
func CompareWinCount(p Params, agg []*londobell.MinerWinCount, pg []*bo.AccWinCount, maxEx MaxExamples) Result {
	r := Result{
		Endpoint:   EndpointWinCount,
		Params:     p,
		AggRows:    len(agg),
		PgRows:     len(pg),
		Unresolved: []string{"TotalGasReward（PG 无对应列；PG 路径恒返回 0）"},
	}
	c := newRecorder(maxEx.limit())

	pgByMiner := make(map[string]*bo.AccWinCount, len(pg))
	for _, row := range pg {
		pgByMiner[normalizeMiner(row.Miner)] = row
		r.PgSumWin += row.WinCount
	}

	seen := make(map[string]bool, len(agg))
	for _, item := range agg {
		if item == nil {
			continue
		}
		r.AggSumWin += item.TotalWinCount
		r.AggSumGas = r.AggSumGas.Add(item.TotalGasReward)

		key := normalizeMiner(item.Id)
		seen[key] = true
		row, ok := pgByMiner[key]
		if !ok {
			c.recordAggOnly(key)
			continue
		}
		c.recordInt64(key, "TotalWinCount", item.TotalWinCount, row.WinCount)
		// TotalGasReward 在 PG 无来源：显式记一条「数值不等」的差异，
		// 不给「悄悄用 0 顶过去」的机会（聚合器侧本来就是 0 时不记）。
		if !item.TotalGasReward.IsZero() {
			c.record(key, "TotalGasReward", item.TotalGasReward.String(), "0 (PG 无来源)", false, true)
		}
	}
	for _, row := range pg {
		k := normalizeMiner(row.Miner)
		if !seen[k] {
			c.recordPgOnly(k)
		}
	}
	c.finish(&r)
	return r
}

func minerEpochKey(epoch int64, miner string) string {
	return fmt.Sprintf("%d|%s", epoch, normalizeMiner(miner))
}

// normalizeMiner 把地址统一成带前缀形态：聚合器的 _id.Miner / Params.Miner 是不带前缀的 0…，
// PG 里存的是带前缀的 f0…（写路径用的是 SmartAddress.Address()）。
func normalizeMiner(addr string) string {
	if addr == "" {
		return ""
	}
	return chain.SmartAddress(addr).Address()
}
