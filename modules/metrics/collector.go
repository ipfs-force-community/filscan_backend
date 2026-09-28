package metrics

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 采集错误作用域（scope 标签取值）。
const (
	ScopeHead   = "head"   // 链头高度
	ScopeSyncer = "syncer" // 同步器游标（chain.sync_syncers）
	ScopeTable  = "table"  // 关键表新鲜度
)

// 采集错误原因（reason 标签取值）。
const (
	ReasonTimeout      = "timeout"       // 超时（含 context deadline）
	ReasonTableMissing = "table_missing" // 表不存在 / 权限不足
	ReasonQueryFailed  = "query_failed"  // 其它查询错误
	ReasonEmpty        = "empty"         // 查询成功但表为空（max 为 NULL）
)

// 指标名。全部使用 filscan_ 前缀，单位一律是「链高度（epoch）」，
// 除 filscan_chain_head_block_time_seconds 之外不做任何时间换算。
const (
	metricUp              = "filscan_metrics_up"
	metricCycles          = "filscan_metrics_collect_cycles_total"
	metricErrors          = "filscan_metrics_collect_errors_total"
	metricHeadHeight      = "filscan_chain_head_height"
	metricHeadBlockTime   = "filscan_chain_head_block_time_seconds"
	metricSyncerCursor    = "filscan_syncer_cursor_height"
	metricSyncerLag       = "filscan_syncer_lag_height"
	metricLegacySyncerLag = "filscan_syncer_delay_height" // 兼容旧规则名（同口径同值，见 Collector.legacyDelayName）
	metricTableCursor     = "filscan_table_cursor_height"
	metricTableLag        = "filscan_table_lag_height"
)

const (
	helpUp              = "1 表示本次采集成功取到链头高度；0 表示链头高度不可用（此时所有 lag 指标不产出，规则需用 absent()/up==0 判「数据源挂了」）。"
	helpCycles          = "采集器已完成的采集轮数（含失败轮次），用于确认采集进程活着。"
	helpErrors          = "采集错误累计次数。单点失败（表不存在/超时/链头不可用）只在这里增长，不影响其它指标产出。"
	helpHeadHeight      = "链头高度（单位：epoch/高度）。数据源：londobell adapter GET /adapter/epoch。"
	helpHeadBlockTime   = "链头区块时间（Unix 秒）。用于「链头本身是否在前进」的判据。"
	helpSyncerCursor    = "同步器已处理（落库）到的链高度（单位：epoch/高度）。数据源：chain.sync_syncers.epoch，由 syncer 每段高度处理完并落库后写入。"
	helpSyncerLag       = "同步落后高度 = 链头高度 − 该同步器已处理高度（单位：epoch/高度，越大越落后）。这是修正后的口径：不再是任何形式的「增量差/单点采样差」。"
	helpLegacySyncerLag = "DEPRECATED 兼容指标：与 filscan_syncer_lag_height 同一采集源、同一口径、同一数值，仅为尚未改名的旧规则保留。新规则请使用 filscan_syncer_lag_height；两个数据源（本采集器与旧 pushgateway 推送脚本）不得同时喂同一个指标名。"
	helpTableCursor     = "关键表停在多少高度 = 该表高度列的最大值 max(高度列)（单位：epoch/高度）。"
	helpTableLag        = "表新鲜度落后高度 = 链头高度 − 该表 max(高度列)（单位：epoch/高度）。直接回答「哪张表停在多少高度、落后多少」。"
)

// SyncerCursor 同步器游标：chain.sync_syncers 中的一条 (name, epoch)。
type SyncerCursor struct {
	Name  string
	Epoch int64
}

// Querier 采集所需的数据源抽象。生产实现见 DBQuerier；单测注入假实现，无需数据库。
type Querier interface {
	// HeadHeight 返回链头高度与链头区块时间。实现失败时必须返回 error（调用方据此停发 lag 指标）。
	HeadHeight(ctx context.Context) (height int64, blockTime time.Time, err error)
	// Syncers 返回全部同步器游标。
	Syncers(ctx context.Context) ([]SyncerCursor, error)
	// MaxHeight 返回某表高度列的最大值；found=false 表示查询成功但表为空（max 为 NULL）。
	MaxHeight(ctx context.Context, spec TableSpec) (height int64, found bool, err error)
}

// ErrorCounter 采集错误计数器：跨轮次累计，渲染为 counter 样本。
type ErrorCounter struct {
	mu sync.Mutex
	m  map[errorKey]float64
}

type errorKey struct {
	Scope  string
	Table  string
	Reason string
}

// NewErrorCounter 新建计数器。
func NewErrorCounter() *ErrorCounter {
	return &ErrorCounter{m: make(map[errorKey]float64)}
}

// Inc 累加一次错误。
func (c *ErrorCounter) Inc(scope, table, reason string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[errorKey{Scope: scope, Table: table, Reason: reason}]++
}

type errorSample struct {
	key   errorKey
	value float64
}

// Snapshot 返回按 (scope, table, reason) 排序的快照，保证渲染稳定。
func (c *ErrorCounter) Snapshot() []errorSample {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	out := make([]errorSample, 0, len(c.m))
	for k, v := range c.m {
		out = append(out, errorSample{key: k, value: v})
	}
	c.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].key.Scope != out[j].key.Scope {
			return out[i].key.Scope < out[j].key.Scope
		}
		if out[i].key.Table != out[j].key.Table {
			return out[i].key.Table < out[j].key.Table
		}
		return out[i].key.Reason < out[j].key.Reason
	})
	return out
}

// Collector 「同步落后高度 + 关键表新鲜度」采集器。
type Collector struct {
	q       Querier
	network string
	tables  []TableSpec
	errors  *ErrorCounter
	cycles  atomic.Uint64

	// legacyDelayName 为 true 时额外输出旧指标名 filscan_syncer_delay_height
	// （与 filscan_syncer_lag_height 同口径同值），供尚未改名的旧规则平滑过渡。
	legacyDelayName bool
}

// Option 采集器可选配置。
type Option func(*Collector)

// WithLegacyDelayName 控制是否输出旧指标名 filscan_syncer_delay_height。
func WithLegacyDelayName(enable bool) Option {
	return func(c *Collector) { c.legacyDelayName = enable }
}

// WithErrorCounter 注入已有错误计数器（便于多次构造共享同一计数器；默认自建）。
func WithErrorCounter(ec *ErrorCounter) Option {
	return func(c *Collector) {
		if ec != nil {
			c.errors = ec
		}
	}
}

// NewCollector 新建采集器。tables 为空时使用 DefaultTableSpecs()。
func NewCollector(q Querier, network string, tables []TableSpec, opts ...Option) *Collector {
	c := &Collector{
		q:               q,
		network:         network,
		tables:          tables,
		errors:          NewErrorCounter(),
		legacyDelayName: true,
	}
	if len(c.tables) == 0 {
		c.tables = DefaultTableSpecs()
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Errors 返回错误计数器（供测试与外部复用）。
func (c *Collector) Errors() *ErrorCounter {
	return c.errors
}

// Tables 返回当前关键表清单。
func (c *Collector) Tables() []TableSpec {
	return c.tables
}

// Collect 执行一次采集并返回指标集合。
//
// 口径（务必与 HELP 文本保持一致，单位全部是「高度/epoch」）：
//
//	filscan_chain_head_height{network}         链头高度
//	filscan_syncer_cursor_height{syncer,network}  同步器已处理高度（chain.sync_syncers.epoch）
//	filscan_syncer_lag_height{syncer,network}     链头高度 − 同步器已处理高度
//	filscan_table_cursor_height{table,column,network} 某表 max(高度列)
//	filscan_table_lag_height{table,column,network}    链头高度 − 某表 max(高度列)
//
// 其中 table 标签取 "schema.table"（如 "fevm.evm_transfers"），column 取高度列名
// （如 "epoch"）。syncer 标签取 chain.sync_syncers.name（如 "chain" / "miner"）。
//
// 容错约定：
//   - 链头取不到 ⇒ filscan_metrics_up=0，且不产出任何 lag 指标（宁缺毋滥：
//     缺数据可以用 absent() 告警，算错的 lag 会让规则永久失效）；
//   - 某张表查询失败/为空 ⇒ 只累加 filscan_metrics_collect_errors_total，该表系列缺失，其余照常；
//   - 同步器游标表查询失败 ⇒ 只累加计数，表新鲜度部分照常产出。
func (c *Collector) Collect(ctx context.Context) *Set {
	set := NewSet()
	c.cycles.Add(1)

	netLabels := map[string]string{"network": c.network}

	// 1) 链头高度（lag 的基准，必须独立于本仓同步器的产物）
	head, blockTime, headErr := c.q.HeadHeight(ctx)
	if headErr != nil {
		c.errors.Inc(ScopeHead, "", reasonOf(headErr))
		_ = set.Add(metricUp, helpUp, Gauge, netLabels, 0)
	} else {
		_ = set.Add(metricUp, helpUp, Gauge, netLabels, 1)
		_ = set.Add(metricHeadHeight, helpHeadHeight, Gauge, netLabels, float64(head))
		if !blockTime.IsZero() {
			_ = set.Add(metricHeadBlockTime, helpHeadBlockTime, Gauge, netLabels, float64(blockTime.Unix()))
		}
	}

	// 2) 同步器游标 ⇒ 单同步器的落后高度
	cursors, err := c.q.Syncers(ctx)
	if err != nil {
		c.errors.Inc(ScopeSyncer, "chain.sync_syncers", reasonOf(err))
	} else {
		for _, cu := range dedupeCursors(cursors) {
			labels := map[string]string{"syncer": cu.Name, "network": c.network}
			_ = set.Add(metricSyncerCursor, helpSyncerCursor, Gauge, labels, float64(cu.Epoch))
			if headErr != nil {
				continue
			}
			lag := float64(head - cu.Epoch)
			_ = set.Add(metricSyncerLag, helpSyncerLag, Gauge, labels, lag)
			if c.legacyDelayName {
				_ = set.Add(metricLegacySyncerLag, helpLegacySyncerLag, Gauge, labels, lag)
			}
		}
	}

	// 3) 关键表新鲜度 ⇒ 哪张表停在多少高度、落后多少
	for _, spec := range c.tables {
		if err := spec.Validate(); err != nil {
			c.errors.Inc(ScopeTable, spec.Qualified(), ReasonQueryFailed)
			continue
		}
		height, found, err := c.q.MaxHeight(ctx, spec)
		if err != nil {
			c.errors.Inc(ScopeTable, spec.Qualified(), reasonOf(err))
			continue
		}
		if !found {
			// 表存在但一行没有：无法给出高度，明确计入错误计数以便区分「空表」与「落后 0」。
			c.errors.Inc(ScopeTable, spec.Qualified(), ReasonEmpty)
			continue
		}
		labels := map[string]string{
			"table":   spec.Qualified(),
			"column":  spec.Column,
			"network": c.network,
		}
		_ = set.Add(metricTableCursor, helpTableCursor, Gauge, labels, float64(height))
		if headErr == nil {
			_ = set.Add(metricTableLag, helpTableLag, Gauge, labels, float64(head-height))
		}
	}

	// 4) 采集器自身可观测性
	_ = set.Add(metricCycles, helpCycles, Counter, netLabels, float64(c.cycles.Load()))
	for _, es := range c.errors.Snapshot() {
		labels := map[string]string{
			"scope":   es.key.Scope,
			"table":   es.key.Table,
			"reason":  es.key.Reason,
			"network": c.network,
		}
		_ = set.Add(metricErrors, helpErrors, Counter, labels, es.value)
	}

	return set
}

// dedupeCursors 按同步器名去重（同名取最大高度）并按名称排序。
//
// 为什么必须去重：chain.sync_syncers 表上没有 (name) 唯一索引
// （见 migration/11.syncer.sql：只有 name/epoch 两列），历史脏数据可能留下
// 同名多行；同名同标签的重复样本会让 Prometheus 整轮抓取失败。
func dedupeCursors(in []SyncerCursor) []SyncerCursor {
	byName := make(map[string]int64, len(in))
	for _, cu := range in {
		if cu.Name == "" {
			continue
		}
		if cur, ok := byName[cu.Name]; !ok || cu.Epoch > cur {
			byName[cu.Name] = cu.Epoch
		}
	}
	out := make([]SyncerCursor, 0, len(byName))
	for name, epoch := range byName {
		out = append(out, SyncerCursor{Name: name, Epoch: epoch})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// reasonOf 把错误粗分类成 reason 标签（用于告警分流：表缺失 vs 超时 vs 其它）。
func reasonOf(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "canceling statement"):
		return ReasonTimeout
	case strings.Contains(msg, "does not exist"),
		strings.Contains(msg, "no such table"),
		strings.Contains(msg, "undefined table"),
		strings.Contains(msg, "permission denied"):
		return ReasonTableMissing
	default:
		return ReasonQueryFailed
	}
}
