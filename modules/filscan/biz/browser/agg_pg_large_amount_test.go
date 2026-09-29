package browser

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	logging "github.com/gozelle/logger"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

// fakeLargeAgg 只实现本文件用到的 TransferLargeAmount，其余经 embedded interface 占位
// （测试不会调用它们；调用即 panic，属于测试本身写错）。
type fakeLargeAgg struct {
	londobell.Agg

	calls   int
	filters types.Filters
	result  *londobell.TransferLargeAmountList
	err     error
}

func (f *fakeLargeAgg) TransferLargeAmount(_ context.Context, filters types.Filters) (*londobell.TransferLargeAmountList, error) {
	f.calls++
	f.filters = filters
	return f.result, f.err
}

// fakeLargeReader 注入的假读实现：记录入参（offset/limit、是否带超时），返回预设结果。
type fakeLargeReader struct {
	pageCalls  int
	countCalls int
	offset     int64
	limit      int64
	hasTimeout bool

	rows  []*bo.LargeTransferRow
	total int64
	err   error
	// countErr 只让计数失败（列表成功）—— 用于验证「TotalCount 缺失也不做降级，整条回落」。
	countErr error
}

func (f *fakeLargeReader) LargeTransfersPage(ctx context.Context, offset, limit int64) ([]*bo.LargeTransferRow, error) {
	f.pageCalls++
	f.offset, f.limit = offset, limit
	if _, ok := ctx.Deadline(); ok {
		f.hasTimeout = true
	}
	return f.rows, f.err
}

func (f *fakeLargeReader) CountLargeTransfers(_ context.Context) (int64, error) {
	f.countCalls++
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.total, f.err
}

func boolPtrLarge(v bool) *bool { return &v }

// captureBizLogs 把 biz 包（github.com/gozelle/logger）的日志抓到内存里，供「必须打 Warn」这类断言使用。
// 默认等级是 Error（Warn 会被过滤），故同时把等级放到 Debug；cleanup 用 SetupLogging(GetConfig()) 复原。
func captureBizLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	logging.SetPrimaryCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(buf),
		zapcore.DebugLevel,
	))
	logging.SetAllLoggers(logging.LevelDebug)
	t.Cleanup(func() { logging.SetupLogging(logging.GetConfig()) })
	return buf
}

// 开关关闭时必须原样返回（同一个 agg 实例）、一次 SQL 都不发 —— 「零行为变化」的硬保证。
func TestPgLargeAmountDisabledPassesThrough(t *testing.T) {
	agg := &fakeLargeAgg{}
	reader := &fakeLargeReader{err: errors.New("reader 不该被调用")}
	got := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{})
	if got != londobell.Agg(agg) {
		t.Fatalf("开关关闭时应原样返回入参 agg，得到 %T", got)
	}

	// 配置入口（老配置文件：没有 [feature] 段）同样原样返回。
	if fromConf := NewPgLargeAmountAgg(agg, &gorm.DB{}, &config.Config{}); fromConf != londobell.Agg(agg) {
		t.Fatalf("配置无 [feature] 段时应原样返回入参 agg，得到 %T", fromConf)
	}
	// 开关开着但 db 为空（单测/未注入）时也不得装饰：宁可走聚合器。
	conf := &config.Config{Feature: &config.Feature{LargeAmountReadFromPg: boolPtrLarge(true)}}
	if got := NewPgLargeAmountAgg(agg, nil, conf); got != londobell.Agg(agg) {
		t.Fatalf("db=nil 时应原样返回入参 agg，得到 %T", got)
	}

	filters := types.Filters{Index: 3, Limit: 20}
	res, err := got.TransferLargeAmount(context.Background(), filters)
	if err != nil {
		t.Fatalf("透传失败: %s", err)
	}
	if res != nil {
		t.Fatalf("透传应返回聚合器结果，得到 %+v", res)
	}
	if agg.calls != 1 || agg.filters != filters {
		t.Errorf("应把 filters 原样透传给聚合器一次，calls=%d filters=%+v", agg.calls, agg.filters)
	}
	if reader.pageCalls != 0 || reader.countCalls != 0 {
		t.Errorf("开关关闭时不应发任何 SQL，pageCalls=%d countCalls=%d", reader.pageCalls, reader.countCalls)
	}
}

// 开关打开：字段逐个对齐（含大额 attoFIL 文本不丢精度、depth/from/to 正确、SignedCid 与 ExitCode 保持零值）。
func TestPgLargeAmountMapping(t *testing.T) {
	agg := &fakeLargeAgg{err: errors.New("开关打开时不该调用聚合器")}
	// 1e22 attoFIL（FIL>=10000 的门槛值）与 38 位极值：过一次 float64 都会失真。
	const (
		thresholdValue = "10000000000000000000000"
		maxValue       = "99999999999999999999999999999999999999"
	)
	reader := &fakeLargeReader{
		total: 245116,
		rows: []*bo.LargeTransferRow{
			{
				Epoch: 6409152, Cid: "bafy2bzaced-signed", RootCid: "bafy2bzaced-root",
				FromAddr: "f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa",
				ToAddr:   "f0443578", Value: decimal.RequireFromString(thresholdValue),
				Method: "Send", Depth: 1,
			},
			{
				Epoch: 6409152, Cid: "bafy2bzaced-signed2", RootCid: "",
				FromAddr: "f1jqwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa7xgyi",
				ToAddr:   "f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa",
				Value:    decimal.RequireFromString(maxValue),
				Method:   "InvokeContract", Depth: 3,
			},
		},
	}
	wrapped := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{LargeAmount: true, Timeout: time.Second})

	res, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{Index: 2, Limit: 50})
	if err != nil {
		t.Fatalf("读 PG 失败: %s", err)
	}
	if agg.calls != 0 {
		t.Errorf("开关打开时不应回落到聚合器，实际调用 %d 次", agg.calls)
	}
	if reader.pageCalls != 1 || reader.countCalls != 1 {
		t.Errorf("应各读一次列表与计数，pageCalls=%d countCalls=%d", reader.pageCalls, reader.countCalls)
	}
	// index 是页码：线上 skip = index*limit（londobell multi-query/query.go:296-300）。
	if reader.offset != 100 || reader.limit != 50 {
		t.Errorf("PG 分页应为 offset=100 limit=50（index=2,limit=50），得到 offset=%d limit=%d", reader.offset, reader.limit)
	}
	if !reader.hasTimeout {
		t.Error("Timeout>0 时 PG 读必须带 deadline（复用 PgReadTimeoutMs 语义）")
	}
	if res == nil {
		t.Fatal("不应返回 nil")
	}
	if res.TotalCount != 245116 {
		t.Errorf("TotalCount 应来自全表 count(*)=245116，得到 %d", res.TotalCount)
	}
	if len(res.TransferLargeAmount) != 2 {
		t.Fatalf("应有 2 行，得到 %d", len(res.TransferLargeAmount))
	}

	first := res.TransferLargeAmount[0]
	if first.Epoch != 6409152 || first.Cid != "bafy2bzaced-signed" || first.RootCid != "bafy2bzaced-root" {
		t.Errorf("Epoch/Cid/RootCid 映射错误: %+v", first)
	}
	if first.From != chain.SmartAddress("f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa") {
		t.Errorf("From 必须原样（robust 地址原文），得到 %q", string(first.From))
	}
	if first.To != chain.SmartAddress("f0443578") {
		t.Errorf("To 必须原样，得到 %q", string(first.To))
	}
	if first.Value.String() != thresholdValue {
		t.Errorf("Value 必须原样（attoFIL 大整数，不过浮点），得到 %s", first.Value.String())
	}
	if first.Method != "Send" || first.Depth != 1 {
		t.Errorf("Method/Depth 映射错误: %+v", first)
	}
	// 聚合器管线的 $project 里没有这三个键 ⇒ 必须与聚合器一样保持零值。
	if first.SignedCid != "" {
		t.Errorf("SignedCid 无对应列（并进了 Cid），必须为空，得到 %q", first.SignedCid)
	}
	if first.ExitCode != 0 {
		t.Errorf("ExitCode 无对应列（$match 已限定 0），必须为 0，得到 %d", first.ExitCode)
	}

	second := res.TransferLargeAmount[1]
	if second.Value.String() != maxValue {
		t.Errorf("38 位 attoFIL 不得丢精度，得到 %s", second.Value.String())
	}
	if second.Depth != 3 {
		t.Errorf("Depth 必须取库值 3（ACL 层靠它判 Cid 是否清空），得到 %d", second.Depth)
	}
	if second.RootCid != "" {
		t.Errorf("库内 root_cid 为空串时 RootCid 必须为空，得到 %q", second.RootCid)
	}
}

// index=limit=0：线上不设分页、取全量 ⇒ PG 侧同样不设上限（offset 0 + MaxInt64）。
func TestPgLargeAmountZeroIndexLimitMeansFullScan(t *testing.T) {
	agg := &fakeLargeAgg{}
	reader := &fakeLargeReader{total: 1, rows: []*bo.LargeTransferRow{{Epoch: 1, Cid: "c"}}}
	wrapped := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{LargeAmount: true})

	if _, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{}); err != nil {
		t.Fatalf("读 PG 失败: %s", err)
	}
	if reader.offset != 0 || reader.limit <= 0 || reader.limit == 0 {
		t.Errorf("index=limit=0 应折算成 offset=0 且不设上限，得到 offset=%d limit=%d", reader.offset, reader.limit)
	}
	if reader.limit < 1<<40 {
		t.Errorf("index=limit=0 的 limit 应为「不设上限」的哨兵值，得到 %d", reader.limit)
	}
}

// 列表读报错（表缺失/超时/SQL 错）⇒ 回落聚合器 + 打 Warn（宁慢不空）。
func TestPgLargeAmountListErrorFallsBackWithWarn(t *testing.T) {
	logs := captureBizLogs(t)
	agg := &fakeLargeAgg{result: &londobell.TransferLargeAmountList{TotalCount: 7}}
	reader := &fakeLargeReader{err: errors.New(`ERROR: relation "chain.large_transfers" does not exist`)}
	wrapped := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{LargeAmount: true})

	res, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{Index: 0, Limit: 20})
	if err != nil {
		t.Fatalf("应回落聚合器而不是把错误抛给上游: %s", err)
	}
	if agg.calls != 1 || res == nil || res.TotalCount != 7 {
		t.Errorf("应回落聚合器拿到结果，calls=%d res=%+v", agg.calls, res)
	}
	out := logs.String()
	if !strings.Contains(out, "large amount transfers from pg failed") || !strings.Contains(out, "fallback to aggregator") {
		t.Errorf("PG 读失败必须打 Warn 并说明回落，实际日志:\n%s", out)
	}
	if !strings.Contains(out, "warn") && !strings.Contains(out, "WARN") {
		t.Errorf("日志等级必须是 Warn（便于告警检索），实际日志:\n%s", out)
	}
}

// 计数失败同样整条回落：TotalCount 缺失会让前端页码错乱，不做「列表用 PG、计数补 0」的降级。
func TestPgLargeAmountCountErrorFallsBack(t *testing.T) {
	logs := captureBizLogs(t)
	fallback := &londobell.TransferLargeAmountList{TotalCount: 245116}
	agg := &fakeLargeAgg{result: fallback}
	reader := &fakeLargeReader{total: 245116, rows: []*bo.LargeTransferRow{{Epoch: 1, Cid: "c"}}, countErr: errors.New("context deadline exceeded")}
	wrapped := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{LargeAmount: true})

	res, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{Index: 0, Limit: 20})
	if err != nil {
		t.Fatalf("应回落聚合器: %s", err)
	}
	if agg.calls != 1 || res != fallback {
		t.Errorf("应整条回落聚合器（拿到聚合器自己的结果），calls=%d res=%+v", agg.calls, res)
	}
	if !strings.Contains(logs.String(), "count large amount transfers from pg failed") {
		t.Errorf("计数失败必须打 Warn，实际日志:\n%s", logs.String())
	}
}

// 空页 = nil（聚合器此时回 data:null，bindResult 把 nil 交给调用方、TotalCount 同样不返回）。
func TestPgLargeAmountEmptyPageIsNil(t *testing.T) {
	agg := &fakeLargeAgg{}
	reader := &fakeLargeReader{total: 245116}
	wrapped := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{LargeAmount: true})

	res, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{Index: 99999, Limit: 20})
	if err != nil {
		t.Fatalf("空页不是错误: %s", err)
	}
	if res != nil {
		t.Errorf("空页必须返回 nil（与聚合器 data:null 同形态），得到 %+v", res)
	}
	if agg.calls != 0 {
		t.Errorf("空页不应回落聚合器（会白白再挂一次），calls=%d", agg.calls)
	}
	if reader.countCalls != 0 {
		t.Errorf("空页不必再查计数，countCalls=%d", reader.countCalls)
	}
}

// Timeout<=0（配置显式关闭超时）时不得给 ctx 加 deadline。
func TestPgLargeAmountNoTimeoutWhenDisabled(t *testing.T) {
	agg := &fakeLargeAgg{}
	reader := &fakeLargeReader{total: 1, rows: []*bo.LargeTransferRow{{Epoch: 1, Cid: "c"}}}
	wrapped := NewPgLargeAmountAggWithReader(agg, reader, PgLargeAmountOptions{LargeAmount: true})

	if _, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{Index: 0, Limit: 1}); err != nil {
		t.Fatalf("读 PG 失败: %s", err)
	}
	if reader.hasTimeout {
		t.Error("Timeout<=0 表示不设超时，不应给 ctx 加 deadline")
	}
}

// 与三个统计端点的开关互不影响：两个装饰器可以叠在一起，各自只接管自己的端点数。
func TestPgLargeAmountSwitchIndependentFromRewardSwitches(t *testing.T) {
	laAgg := &fakeLargeAgg{}
	laReader := &fakeLargeReader{total: 3, rows: []*bo.LargeTransferRow{{Epoch: 1, Cid: "c"}}}
	wrapped := NewPgLargeAmountAggWithReader(laAgg, laReader, PgLargeAmountOptions{LargeAmount: true})

	// 未开启的端点（这里用 WinCount 代表）仍走嵌入的 agg（fakeLargeAgg 未实现 ⇒ nil 指针 panic 才算透传失败，
	// 故这里只断言开启的那个端点确实走了 PG 且聚合器没被调用）。
	if _, err := wrapped.TransferLargeAmount(context.Background(), types.Filters{Index: 0, Limit: 1}); err != nil {
		t.Fatalf("读 PG 失败: %s", err)
	}
	if laAgg.calls != 0 {
		t.Errorf("开启的端点不应回落聚合器，calls=%d", laAgg.calls)
	}
	// 配置驱动的开关解析（含 [feature] 段存在但字段缺失 = 关闭）。
	off := NewPgLargeAmountAgg(laAgg, &gorm.DB{}, &config.Config{Feature: &config.Feature{}})
	if off != londobell.Agg(laAgg) {
		t.Error("[feature] 段存在但未配 large_amount_read_from_pg 时应原样返回")
	}
}
