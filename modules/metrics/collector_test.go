package metrics

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeQuerier 假数据源：单测无需数据库/网络。
type fakeQuerier struct {
	head       int64
	blockTime  time.Time
	headErr    error
	cursors    []SyncerCursor
	syncersErr error
	max        map[string]int64 // key: "schema.table:column"
	maxErr     map[string]error // key: "schema.table:column"
	empty      map[string]bool  // key: "schema.table:column"
	// maxCalls 记录每个规格被查询的次数（用于验证缓存只打一次库）
	maxCalls map[string]int
}

func newFakeQuerier(head int64) *fakeQuerier {
	return &fakeQuerier{
		head:      head,
		blockTime: time.Unix(1758960000, 0),
		max:       map[string]int64{},
		maxErr:    map[string]error{},
		empty:     map[string]bool{},
		maxCalls:  map[string]int{},
	}
}

func (f *fakeQuerier) HeadHeight(context.Context) (int64, time.Time, error) {
	if f.headErr != nil {
		return 0, time.Time{}, f.headErr
	}
	return f.head, f.blockTime, nil
}

func (f *fakeQuerier) Syncers(context.Context) ([]SyncerCursor, error) {
	if f.syncersErr != nil {
		return nil, f.syncersErr
	}
	return f.cursors, nil
}

func (f *fakeQuerier) MaxHeight(_ context.Context, spec TableSpec) (int64, bool, error) {
	key := spec.String()
	f.maxCalls[key]++
	if err := f.maxErr[key]; err != nil {
		return 0, false, err
	}
	if f.empty[key] {
		return 0, false, nil
	}
	v, ok := f.max[key]
	if !ok {
		return 0, false, nil
	}
	return v, true, nil
}

func specsOf(t *testing.T, list ...string) []TableSpec {
	t.Helper()
	specs, err := ParseTableSpecs(list)
	require.NoError(t, err)
	return specs
}

func sampleValue(t *testing.T, set *Set, name string, labels map[string]string) float64 {
	t.Helper()
	m, ok := set.index[name]
	require.True(t, ok, "指标 %s 缺失", name)
	for _, s := range m.Samples {
		if labelKey(s.Labels) == labelKey(labels) {
			return s.Value
		}
	}
	require.Failf(t, "样本缺失", "指标 %s 缺少标签 %s（现有：%s）", name, labelKey(labels), setDebug(m))
	return 0
}

func sampleMissing(set *Set, name string, labels map[string]string) bool {
	m, ok := set.index[name]
	if !ok {
		return true
	}
	for _, s := range m.Samples {
		if labelKey(s.Labels) == labelKey(labels) {
			return false
		}
	}
	return true
}

func setDebug(m *Metric) string {
	out := ""
	for _, s := range m.Samples {
		out += fmt.Sprintf("[%s=%v] ", labelKey(s.Labels), s.Value)
	}
	return out
}

// TestCollectIncidentScenario 复现 2026-09 主网静默停摆：链头 6408450，
// 游标停在 6357057/6357103 ⇒ 落后 51393/51347，远超旧规则阈值 720。
// 这是对「口径修正」最关键的一条回归断言：新口径必须报出这个量级。
func TestCollectIncidentScenario(t *testing.T) {
	const head = int64(6408450)
	f := newFakeQuerier(head)
	f.cursors = []SyncerCursor{
		{Name: "chain", Epoch: 6357057},
		{Name: "miner", Epoch: 6357103},
	}
	f.max = map[string]int64{
		"chain.sync_syncers:epoch":  6357103,
		"chain.actor_actions:epoch": 6357057,
		"fevm.evm_transfers:epoch":  6356000,
	}
	c := NewCollector(f, "mainnet", specsOf(t,
		"chain.sync_syncers:epoch", "chain.actor_actions:epoch", "fevm.evm_transfers:epoch"))

	set := c.Collect(context.Background())

	require.Equal(t, float64(1), sampleValue(t, set, metricUp, map[string]string{"network": "mainnet"}))
	require.Equal(t, float64(head), sampleValue(t, set, metricHeadHeight, map[string]string{"network": "mainnet"}))

	// 单同步器落后高度（修正后的口径：链头 − 该同步器已处理高度）
	require.Equal(t, float64(51393), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, float64(51347), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "miner", "network": "mainnet"}))

	// 表新鲜度（链头 − 表 max(epoch)）
	require.Equal(t, float64(51393), sampleValue(t, set, metricTableLag, map[string]string{
		"table": "chain.actor_actions", "column": "epoch", "network": "mainnet"}))
	require.Equal(t, float64(52450), sampleValue(t, set, metricTableLag, map[string]string{
		"table": "fevm.evm_transfers", "column": "epoch", "network": "mainnet"}))
	require.Equal(t, float64(6356000), sampleValue(t, set, metricTableCursor, map[string]string{
		"table": "fevm.evm_transfers", "column": "epoch", "network": "mainnet"}))

	// 兼容旧指标名：同口径同值（不为 2/60 那种小数字）
	require.Equal(t, float64(51393), sampleValue(t, set, metricLegacySyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
}

func TestCollectSyncerCursorAndLag(t *testing.T) {
	f := newFakeQuerier(1000)
	f.cursors = []SyncerCursor{{Name: "chain", Epoch: 998}, {Name: "evm", Epoch: 1000}}
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))

	set := c.Collect(context.Background())
	require.Equal(t, float64(998), sampleValue(t, set, metricSyncerCursor,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, float64(2), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, float64(0), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "evm", "network": "mainnet"}))
}

// TestCollectHeadUnavailable 链头取不到时必须 up=0 且**不产出** lag 指标
// （宁缺毋滥：缺数据可以用 absent() 告警，算错的 lag 会让规则永久失效）。
func TestCollectHeadUnavailable(t *testing.T) {
	f := newFakeQuerier(1000)
	f.headErr = errors.New("adapter: connection refused")
	f.cursors = []SyncerCursor{{Name: "chain", Epoch: 900}}
	f.max = map[string]int64{"chain.actor_actions:epoch": 800}
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))

	set := c.Collect(context.Background())

	require.Equal(t, float64(0), sampleValue(t, set, metricUp, map[string]string{"network": "mainnet"}))
	require.True(t, sampleMissing(set, metricHeadHeight, map[string]string{"network": "mainnet"}))
	require.True(t, sampleMissing(set, metricSyncerLag, map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.True(t, sampleMissing(set, metricTableLag, map[string]string{
		"table": "chain.actor_actions", "column": "epoch", "network": "mainnet"}))

	// 但「绝对值」仍然有用，必须照常产出
	require.Equal(t, float64(900), sampleValue(t, set, metricSyncerCursor,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, float64(800), sampleValue(t, set, metricTableCursor, map[string]string{
		"table": "chain.actor_actions", "column": "epoch", "network": "mainnet"}))

	require.Equal(t, float64(1), sampleValue(t, set, metricErrors, map[string]string{
		"scope": ScopeHead, "table": "", "reason": ReasonQueryFailed, "network": "mainnet"}))
}

// TestCollectToleratesMissingTable 单点失败不得影响整轮：一张表不存在，
// 其余表和同步器指标照常产出，并按 reason=table_missing 计数。
func TestCollectToleratesMissingTable(t *testing.T) {
	f := newFakeQuerier(1000)
	f.cursors = []SyncerCursor{{Name: "chain", Epoch: 950}}
	f.max = map[string]int64{"chain.actor_actions:epoch": 940}
	f.maxErr = map[string]error{
		"fevm.evm_transfers:epoch": errors.New(`ERROR: relation "fevm.evm_transfers" does not exist (SQLSTATE 42P01)`),
	}
	c := NewCollector(f, "mainnet", specsOf(t,
		"chain.actor_actions:epoch", "fevm.evm_transfers:epoch", "chain.sync_syncers:epoch"))

	set := c.Collect(context.Background())

	// 存在的表照常
	require.Equal(t, float64(60), sampleValue(t, set, metricTableLag, map[string]string{
		"table": "chain.actor_actions", "column": "epoch", "network": "mainnet"}))
	// 缺失的表：无系列 + 明确计数
	require.True(t, sampleMissing(set, metricTableCursor, map[string]string{
		"table": "fevm.evm_transfers", "column": "epoch", "network": "mainnet"}))
	require.Equal(t, float64(1), sampleValue(t, set, metricErrors, map[string]string{
		"scope": ScopeTable, "table": "fevm.evm_transfers",
		"reason": ReasonTableMissing, "network": "mainnet"}))
	// 同步器部分不受影响
	require.Equal(t, float64(50), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
}

func TestCollectEmptyTableCountsAsEmpty(t *testing.T) {
	f := newFakeQuerier(1000)
	f.empty["chain.actor_actions:epoch"] = true
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))

	set := c.Collect(context.Background())
	require.True(t, sampleMissing(set, metricTableCursor, map[string]string{
		"table": "chain.actor_actions", "column": "epoch", "network": "mainnet"}))
	require.Equal(t, float64(1), sampleValue(t, set, metricErrors, map[string]string{
		"scope": ScopeTable, "table": "chain.actor_actions",
		"reason": ReasonEmpty, "network": "mainnet"}))
}

func TestCollectSyncersErrorDoesNotBlockTables(t *testing.T) {
	f := newFakeQuerier(1000)
	f.syncersErr = context.DeadlineExceeded
	f.max = map[string]int64{"chain.actor_actions:epoch": 990}
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))

	set := c.Collect(context.Background())
	require.Equal(t, float64(10), sampleValue(t, set, metricTableLag, map[string]string{
		"table": "chain.actor_actions", "column": "epoch", "network": "mainnet"}))
	require.Equal(t, float64(1), sampleValue(t, set, metricErrors, map[string]string{
		"scope": ScopeSyncer, "table": "chain.sync_syncers",
		"reason": ReasonTimeout, "network": "mainnet"}))
}

// TestCollectDedupesSyncerNames chain.sync_syncers 没有 name 唯一索引
// （见 migration/11.syncer.sql），同名多行会让 Prometheus 整轮抓取失败。
func TestCollectDedupesSyncerNames(t *testing.T) {
	f := newFakeQuerier(1000)
	f.cursors = []SyncerCursor{
		{Name: "chain", Epoch: 900},
		{Name: "chain", Epoch: 950}, // 同名取最大
		{Name: "", Epoch: 999},      // 空名丢弃
	}
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))
	set := c.Collect(context.Background())

	require.Equal(t, float64(950), sampleValue(t, set, metricSyncerCursor,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, float64(50), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, 1, len(set.index[metricSyncerCursor].Samples))
}

func TestCollectLegacyDelayNameCanBeDisabled(t *testing.T) {
	f := newFakeQuerier(1000)
	f.cursors = []SyncerCursor{{Name: "chain", Epoch: 998}}
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"),
		WithLegacyDelayName(false))
	set := c.Collect(context.Background())
	require.True(t, sampleMissing(set, metricLegacySyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
	require.Equal(t, float64(2), sampleValue(t, set, metricSyncerLag,
		map[string]string{"syncer": "chain", "network": "mainnet"}))
}

func TestCollectCountsCyclesAndErrorsAcrossRounds(t *testing.T) {
	f := newFakeQuerier(1000)
	f.maxErr["chain.actor_actions:epoch"] = errors.New("boom")
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))

	c.Collect(context.Background())
	set := c.Collect(context.Background())

	require.Equal(t, float64(2), sampleValue(t, set, metricCycles, map[string]string{"network": "mainnet"}))
	require.Equal(t, float64(2), sampleValue(t, set, metricErrors, map[string]string{
		"scope": ScopeTable, "table": "chain.actor_actions",
		"reason": ReasonQueryFailed, "network": "mainnet"}))
}

func TestCollectDefaultsToBuiltinTables(t *testing.T) {
	f := newFakeQuerier(1000)
	c := NewCollector(f, "mainnet", nil)
	require.Equal(t, len(DefaultTableSpecs()), len(c.Tables()))
	_ = c.Collect(context.Background())
	require.Equal(t, len(DefaultTableSpecs()), len(f.maxCalls))
}

func TestReasonOf(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, ReasonTimeout},
		{context.Canceled, ReasonTimeout},
		{errors.New("pq: canceling statement due to statement timeout"), ReasonTimeout},
		{errors.New(`relation "chain.foo" does not exist`), ReasonTableMissing},
		{errors.New("permission denied for table foo"), ReasonTableMissing},
		{errors.New("boom"), ReasonQueryFailed},
	}
	for _, c := range cases {
		require.Equal(t, c.want, reasonOf(c.err), "%v", c.err)
	}
	require.Equal(t, "", reasonOf(nil))
}

func TestErrorCounterSnapshotSorted(t *testing.T) {
	ec := NewErrorCounter()
	ec.Inc(ScopeTable, "b.t", ReasonEmpty)
	ec.Inc(ScopeTable, "a.t", ReasonEmpty)
	ec.Inc(ScopeHead, "", ReasonQueryFailed)

	snaps := ec.Snapshot()
	require.Len(t, snaps, 3)
	require.Equal(t, ScopeHead, snaps[0].key.Scope)
	require.Equal(t, "a.t", snaps[1].key.Table)
	require.Equal(t, "b.t", snaps[2].key.Table)
}
