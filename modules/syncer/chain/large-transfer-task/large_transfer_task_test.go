package large_transfer_task

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件全部离线：不连数据库、不连聚合器/适配器。写入侧用假仓储（内存里复刻
// 「同一事务内先 delete 后 insert」的语义），断言只落在「到底发了哪些写操作、表里最终是什么」。
// DAL 真的把 delete+insert 放进同一个事务这一条，由 modules/common/infra/dal 的单测用假连接断言。

const (
	testEpoch int64 = 6409152
	nextEpoch int64 = 6409153
	atto1e22        = "10000000000000000000000" // 10000 FIL：线上的命中下界（含）
	attoBelow       = "9999999999999999999999"  // 1e22 - 1：不命中
)

// fakeRepo 假仓储：内存里维护 chain.large_transfers。
//
// 语义刻意与大型表一致：**一次 ReplaceLargeTransfers = 一个事务**（失败时删除与插入都不生效），
// 因此「重放同一高度结果一致」可以在这一层被断言。
type fakeRepo struct {
	mu    sync.Mutex
	rows  map[int64][]*po.LargeTransfer
	calls []string
	// err 非 nil 时所有写操作都失败（模拟 PG 超时/断连/表不存在）
	err error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[int64][]*po.LargeTransfer{}}
}

func (f *fakeRepo) ReplaceLargeTransfers(_ context.Context, epoch int64, items []*po.LargeTransfer) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, fmt.Sprintf("delete:%d", epoch))
	if f.err != nil {
		// 同一事务语义：任何一步失败 ⇒ 整体回滚（删除也不生效）
		f.calls = append(f.calls, "rollback")
		return f.err
	}

	delete(f.rows, epoch)
	for _, item := range items {
		cp := *item
		f.rows[epoch] = append(f.rows[epoch], &cp)
	}
	f.calls = append(f.calls, fmt.Sprintf("insert:%d:%d", epoch, len(items)))
	return nil
}

func (f *fakeRepo) DeleteLargeTransfersFromEpoch(_ context.Context, gteEpoch chain.Epoch) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, fmt.Sprintf("delete_from:%d", gteEpoch.Int64()))
	if f.err != nil {
		f.calls = append(f.calls, "rollback")
		return f.err
	}
	for e := range f.rows {
		if e >= gteEpoch.Int64() {
			delete(f.rows, e)
		}
	}
	return nil
}

func (f *fakeRepo) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// rowsOf 该高度当前的行（深拷贝，防止断言被后续写入影响）
func (f *fakeRepo) rowsOf(epoch int64) []po.LargeTransfer {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]po.LargeTransfer, 0, len(f.rows[epoch]))
	for _, r := range f.rows[epoch] {
		out = append(out, *r)
	}
	return out
}

// snapshot 全表内容（深拷贝）
func (f *fakeRepo) snapshot() map[int64][]po.LargeTransfer {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int64][]po.LargeTransfer{}
	for e, rows := range f.rows {
		for _, r := range rows {
			out[e] = append(out[e], *r)
		}
	}
	return out
}

// callLog 写操作序列（delete:高度 / insert:高度:行数 / rollback / delete_from:高度）
func (f *fakeRepo) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// cid 造 cid 原文用的辅助
func strPtr(s string) *string { return &s }

// newTrace 造一条与 /aggregators/traces 响应同形的 trace。
//
// 关键点（决定本用例是否有意义）：
//   - Msg 子文档带 Value/MethodName（traces.js 的 `Msg: "$Msg"` 整份投影，这两个字段就是线上管道
//     用的 $Msg.Value / $Msg.MethodName）；
//   - Value 是 message 文档的 Value（管道一并返回，用作兜底）；
//   - From/To：Msg 子文档里的地址（线上管道投影的就是 $Msg.From / $Msg.To），
//     响应里是带网络前缀的形态（"f1..."/"t3..."），落到 pg 时要还原成不带前缀的原文。
func newTrace(epoch int64, cid, attoValue, method string, depth int, seq []int64, isBlock bool) *londobell.TraceMessage {
	v := decimal.RequireFromString(attoValue)
	return &londobell.TraceMessage{
		Epoch:   epoch,
		Cid:     cid,
		Depth:   depth,
		Seq:     seq,
		IsBlock: isBlock,
		From:    chain.SmartAddress("f1from"),
		To:      chain.SmartAddress("t3to"),
		Value:   v,
		Msg: &londobell.Msg{
			From: chain.SmartAddress("f1from"), To: chain.SmartAddress("t3to"),
			Value: &v, MethodName: method,
		},
		MsgRct: &londobell.MsgRct{ExitCode: 0},
		Detail: &londobell.MessageDetail{Method: method},
	}
}

func contextWithTraces(t *testing.T, epoch int64, traces any, empty bool) *syncer.Context {
	t.Helper()
	ctx, err := syncer.NewTestContextWithData(nil, nil, chain.Epoch(epoch), empty,
		map[syncer.DataKey]any{syncer.TracesTey: traces})
	require.NoError(t, err)
	return ctx
}

// ---- 筛选逻辑：与线上管道 "MsgRct.ExitCode": 0 && "FIL": {$gte: 10000} 等价 ----

func TestSelectLargeTransfersFilterBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		trace    *londobell.TraceMessage
		selected bool
	}{
		{
			name:     "下界 1e22 attoFIL（= 10000 FIL）命中",
			trace:    newTrace(testEpoch, "bafy-a", atto1e22, "Send", 1, []int64{4}, true),
			selected: true,
		},
		{
			name:     "下界减一（1e22 - 1 attoFIL）不命中",
			trace:    newTrace(testEpoch, "bafy-b", attoBelow, "Send", 1, []int64{4}, true),
			selected: false,
		},
		{
			name:     "下界加一命中（floor(V/1e18) 仍为 10000）",
			trace:    newTrace(testEpoch, "bafy-c", "10000000000000000000001", "Send", 1, []int64{4}, true),
			selected: true,
		},
		{
			name:     "1 FIL 不命中",
			trace:    newTrace(testEpoch, "bafy-d", "1000000000000000000", "Send", 1, []int64{4}, true),
			selected: false,
		},
		{
			name:     "金额为 0 不命中",
			trace:    newTrace(testEpoch, "bafy-e", "0", "Send", 1, []int64{4}, true),
			selected: false,
		},
		{
			name: "ExitCode != 0 即使金额够大也不命中",
			trace: func() *londobell.TraceMessage {
				tr := newTrace(testEpoch, "bafy-f", atto1e22, "Send", 1, []int64{4}, true)
				tr.MsgRct.ExitCode = 21
				return tr
			}(),
			selected: false,
		},
		{
			name: "缺 MsgRct 不命中（mongo 的 MsgRct.ExitCode: 0 不匹配缺字段的文档）",
			trace: func() *londobell.TraceMessage {
				tr := newTrace(testEpoch, "bafy-g", atto1e22, "Send", 1, []int64{4}, true)
				tr.MsgRct = nil
				return tr
			}(),
			selected: false,
		},
		{
			name:     "nil trace 不命中（traces 里夹空元素不再 panic）",
			trace:    nil,
			selected: false,
		},
		{
			name: "Msg.Value 与 message.Value 不一致时以 Msg.Value 为准（线上口径）",
			trace: func() *londobell.TraceMessage {
				tr := newTrace(testEpoch, "bafy-h", "1000000000000000000", "Send", 1, []int64{4}, true)
				big := decimal.RequireFromString(atto1e22)
				tr.Msg.Value = &big
				return tr
			}(),
			selected: true,
		},
		{
			name: "Msg.Value 缺失时退回 message.Value",
			trace: func() *londobell.TraceMessage {
				tr := newTrace(testEpoch, "bafy-i", atto1e22, "Send", 1, []int64{4}, true)
				tr.Msg.Value = nil
				return tr
			}(),
			selected: true,
		},
		{
			name: "缺 Msg 子文档但 message.Value 够大（畸形文档不 panic）",
			trace: func() *londobell.TraceMessage {
				tr := newTrace(testEpoch, "bafy-j", atto1e22, "Send", 1, []int64{4}, true)
				tr.Msg = nil
				return tr
			}(),
			selected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{c.trace})
			if c.selected {
				require.Len(t, rows, 1)
			} else {
				require.Empty(t, rows)
			}
		})
	}
}

// ---- 列映射：与线上管道的 $project 逐列对齐 ----

func TestSelectLargeTransfersColumns(t *testing.T) {
	// 同高度的根消息（IsBlock）+ 两个子调用：SUM 的根 cid 应从根消息还原
	root := newTrace(testEpoch, "bafy-root", "1000000000000000000", "Send", 1, []int64{4}, true)
	root.SignedCid = strPtr("bafy-root-signed")

	child := newTrace(testEpoch, "bafy-child", atto1e22, "InvokeContract", 2, []int64{4, 0}, false)
	child.Msg.Value = ptrDecimal(atto1e22)
	child.Msg.MethodName = "InvokeContract"

	traces := []*londobell.TraceMessage{root, child}
	rows := SelectLargeTransfers(testEpoch, traces)

	require.Len(t, rows, 1, "只有子调用金额过线")
	row := rows[0]
	require.Equal(t, testEpoch, row.Epoch)
	require.Equal(t, "bafy-child", row.Cid, "SignedCid 为空时取 Cid")
	require.NotNil(t, row.RootCid)
	require.Equal(t, "bafy-root-signed", *row.RootCid, "根消息 cid = 根的 SignedCid 优先")
	require.Equal(t, atto1e22, row.Value, "value 是 attoFIL 十进制原文，不经过浮点")
	require.Equal(t, "InvokeContract", row.Method)
	require.Equal(t, 2, row.Depth)
	require.Equal(t, "1from", row.FromAddr, "from_addr 是不带网络前缀的地址原文")
	require.Equal(t, "3to", row.ToAddr)

	t.Run("SignedCid 优先于 Cid", func(t *testing.T) {
		tr := newTrace(testEpoch, "bafy-x", atto1e22, "Send", 1, []int64{7}, true)
		tr.SignedCid = strPtr("bafy-x-signed")
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{tr})
		require.Len(t, got, 1)
		require.Equal(t, "bafy-x-signed", got[0].Cid)
	})

	t.Run("SignedCid 是空串时回退 Cid（不写空 cid）", func(t *testing.T) {
		tr := newTrace(testEpoch, "bafy-y", atto1e22, "Send", 1, []int64{7}, true)
		tr.SignedCid = strPtr("")
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{tr})
		require.Len(t, got, 1)
		require.Equal(t, "bafy-y", got[0].Cid)
	})

	t.Run("地址还原：带网络前缀 ⇒ 去掉首字符；已经是原文 ⇒ 原样，绝不写成空串", func(t *testing.T) {
		cases := []struct {
			name       string
			msgFrom    chain.SmartAddress
			traceFrom  chain.SmartAddress
			wantFromAd string
		}{
			{"Msg 里是带前缀的形态（线上响应常态）", "f1abc", "f1abc", "1abc"},
			{"Msg 里已经是原文（不带前缀）", "1abc", "1abc", "1abc"},
			{"ID 地址同样处理", "f0100", "f0100", "0100"},
			{"Msg 为空 ⇒ 退回 trace 自己的 From", "", "f1fallback", "1fallback"},
		}
		for _, c := range cases {
			trace := newTrace(testEpoch, "bafy-addr", atto1e22, "Send", 1, []int64{21}, true)
			trace.Msg.From = c.msgFrom
			trace.Msg.To = c.msgFrom
			trace.From = c.traceFrom
			trace.To = c.traceFrom
			got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{trace})
			require.Len(t, got, 1)
			require.Equal(t, c.wantFromAd, got[0].FromAddr, c.name)
			require.Equal(t, c.wantFromAd, got[0].ToAddr, c.name)
		}
	})

	t.Run("Msg 缺失时从 trace 的 From/To 取地址", func(t *testing.T) {
		trace := newTrace(testEpoch, "bafy-nomsg", atto1e22, "Send", 1, []int64{23}, true)
		trace.Msg = nil
		trace.From = chain.SmartAddress("f1trace")
		trace.To = chain.SmartAddress("t3trace")
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{trace})
		require.Len(t, got, 1)
		require.Equal(t, "1trace", got[0].FromAddr)
		require.Equal(t, "3trace", got[0].ToAddr)
	})

	t.Run("根消息自身（IsBlock）的 root_cid 为 NULL，与线上投影一致", func(t *testing.T) {
		tr := newTrace(testEpoch, "bafy-top", atto1e22, "Send", 1, []int64{9}, true)
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{tr})
		require.Len(t, got, 1)
		require.Nil(t, got[0].RootCid)
	})

	t.Run("Seq 只有一段但不是 IsBlock 时 root_cid 也为 NULL（mongo 端同样不登记）", func(t *testing.T) {
		tr := newTrace(testEpoch, "bafy-top2", atto1e22, "Send", 1, []int64{9}, false)
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{tr})
		require.Len(t, got, 1)
		require.Nil(t, got[0].RootCid)
	})

	t.Run("子调用但本批 traces 里没有根消息时 root_cid 为 NULL", func(t *testing.T) {
		childOnly := newTrace(testEpoch, "bafy-alone", atto1e22, "Send", 2, []int64{11, 0}, false)
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{childOnly})
		require.Len(t, got, 1)
		require.Nil(t, got[0].RootCid)
	})

	t.Run("根消息的 SignedCid 为空时 root_cid 取根消息的 Cid", func(t *testing.T) {
		r := newTrace(testEpoch, "bafy-plain", "1000000000000000000", "Send", 1, []int64{13}, true)
		c := newTrace(testEpoch, "bafy-sub", atto1e22, "Send", 2, []int64{13, 1}, false)
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{r, c})
		require.Len(t, got, 1)
		require.NotNil(t, got[0].RootCid)
		require.Equal(t, "bafy-plain", *got[0].RootCid)
	})

	t.Run("method 缺失 Msg.MethodName 时退回 Detail.Method", func(t *testing.T) {
		tr := newTrace(testEpoch, "bafy-m", atto1e22, "SubmitWindowedPoSt", 1, []int64{15}, true)
		tr.Msg.MethodName = ""
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{tr})
		require.Len(t, got, 1)
		require.Equal(t, "SubmitWindowedPoSt", got[0].Method)
	})

	t.Run("method 两处都缺时写空串（不写 NULL）", func(t *testing.T) {
		tr := newTrace(testEpoch, "bafy-n", atto1e22, "Send", 1, []int64{15}, true)
		tr.Msg.MethodName = ""
		tr.Detail = nil
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{tr})
		require.Len(t, got, 1)
		require.Equal(t, "", got[0].Method)
	})

	t.Run("同一高度同一 cid 的多条 trace 都保留（线上口径保留行重数）", func(t *testing.T) {
		a := newTrace(testEpoch, "bafy-dup", atto1e22, "Send", 1, []int64{17}, true)
		b := newTrace(testEpoch, "bafy-dup", atto1e22, "Send", 1, []int64{17}, true)
		got := SelectLargeTransfers(testEpoch, []*londobell.TraceMessage{a, b})
		require.Len(t, got, 2, "没有主键/唯一索引，重复 cid 必须保留下重数")
	})
}

func ptrDecimal(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// ---- Exec：写库、幂等、失败不阻断、开关关闭零 SQL ----

func TestExecWritesRowsWithDeleteThenInsert(t *testing.T) {
	repo := newFakeRepo()
	task := NewLargeTransferTask(repo, true)

	traces := []*londobell.TraceMessage{
		newTrace(testEpoch, "bafy-hit", atto1e22, "Send", 1, []int64{4}, true),
		newTrace(testEpoch, "bafy-miss", "1000000000000000000", "Send", 1, []int64{5}, true),
	}
	require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, traces, false)))

	rows := repo.rowsOf(testEpoch)
	require.Len(t, rows, 1)
	require.Equal(t, "bafy-hit", rows[0].Cid)
	require.Equal(t, []string{fmt.Sprintf("delete:%d", testEpoch), fmt.Sprintf("insert:%d:1", testEpoch)}, repo.callLog(),
		"同一高度必须先 delete 再 insert")
	require.Zero(t, task.FailedEpochs())
}

func TestExecIsIdempotentOnReplay(t *testing.T) {
	repo := newFakeRepo()
	task := NewLargeTransferTask(repo, true)
	traces := []*londobell.TraceMessage{
		newTrace(testEpoch, "bafy-hit1", atto1e22, "Send", 1, []int64{4}, true),
		newTrace(testEpoch, "bafy-hit2", "20000000000000000000000", "Send", 1, []int64{5}, true),
	}
	ctx := contextWithTraces(t, testEpoch, traces, false)

	require.NoError(t, task.Exec(ctx))
	once := repo.snapshot()

	// 重放同一高度（同步器重试/回放会重复处理），结果必须与只处理一次完全相同
	for i := 0; i < 3; i++ {
		require.NoError(t, task.Exec(ctx))
		require.Equal(t, once, repo.snapshot(), "重复处理同一高度不得多出行、也不得变内容")
	}

	require.Len(t, repo.rowsOf(testEpoch), 2)
	require.Equal(t,
		[]string{
			fmt.Sprintf("delete:%d", testEpoch), fmt.Sprintf("insert:%d:2", testEpoch),
			fmt.Sprintf("delete:%d", testEpoch), fmt.Sprintf("insert:%d:2", testEpoch),
			fmt.Sprintf("delete:%d", testEpoch), fmt.Sprintf("insert:%d:2", testEpoch),
			fmt.Sprintf("delete:%d", testEpoch), fmt.Sprintf("insert:%d:2", testEpoch),
		}, repo.callLog())
}

func TestExecReplacesPreviouslyWrittenRows(t *testing.T) {
	repo := newFakeRepo()
	task := NewLargeTransferTask(repo, true)

	// 第一次：两条命中
	require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, []*londobell.TraceMessage{
		newTrace(testEpoch, "bafy-1", atto1e22, "Send", 1, []int64{4}, true),
		newTrace(testEpoch, "bafy-2", atto1e22, "Send", 1, []int64{5}, true),
	}, false)))
	require.Len(t, repo.rowsOf(testEpoch), 2)

	// 第二次（如链重组后重跑）：只剩一条命中 ⇒ 该高度整体替换，不能留下陈旧行
	require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, []*londobell.TraceMessage{
		newTrace(testEpoch, "bafy-1", atto1e22, "Send", 1, []int64{4}, true),
	}, false)))
	rows := repo.rowsOf(testEpoch)
	require.Len(t, rows, 1)
	require.Equal(t, "bafy-1", rows[0].Cid)
}

func TestExecWithoutTracesIssuesNoSQL(t *testing.T) {
	t.Run("空高度（SetTracesBuilder 写入 nil）", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, nil, true)))
		require.Empty(t, repo.callLog())
		require.Zero(t, task.NoTracesWarns(), "空高度是正常情况，不该告警")
	})

	t.Run("traces 为空切片（该高度确实没有消息）", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, []*londobell.TraceMessage{}, false)))
		require.Empty(t, repo.callLog(), "traces 为空时刻意不做删除：不从「没有 traces」推断「没有大额转账」")
		require.Zero(t, task.NoTracesWarns())
	})

	t.Run("非空高度的 traces 值为 nil（逗号-ok 断言不 panic）", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, nil, false)))
		require.Empty(t, repo.callLog())
		require.Equal(t, int64(1), task.NoTracesWarns())
	})

	t.Run("traces 类型不对（只 Warn 一次，且不下发 SQL）", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		ctx := contextWithTraces(t, testEpoch, "not-a-trace-list", false)
		require.NoError(t, task.Exec(ctx))
		require.NoError(t, task.Exec(ctx))
		require.Empty(t, repo.callLog())
		require.Equal(t, int64(1), task.NoTracesWarns(), "装配问题只提醒一次，不刷屏")
	})

	t.Run("Datamap 里没有 traces 键", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		ctx, err := syncer.NewTestContextWithData(nil, nil, chain.Epoch(nextEpoch), false, nil)
		require.NoError(t, err)
		require.NoError(t, task.Exec(ctx))
		require.NoError(t, task.Exec(ctx))
		require.Empty(t, repo.callLog())
		require.Equal(t, int64(1), task.NoTracesWarns())
	})

	t.Run("traces 存在但没有命中：仍然执行 delete（清陈旧行）但不 insert", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		epoch := nextEpoch
		require.NoError(t, task.Exec(contextWithTraces(t, epoch, []*londobell.TraceMessage{
			newTrace(epoch, "bafy-small", "1", "Send", 1, []int64{4}, true),
		}, false)))
		require.Equal(t, []string{
			fmt.Sprintf("delete:%d", epoch), fmt.Sprintf("insert:%d:0", epoch),
		}, repo.callLog())
		require.Empty(t, repo.rowsOf(epoch))
	})
}

func TestExecDisabledIssuesNoSQL(t *testing.T) {
	repo := newFakeRepo()
	task := NewLargeTransferTask(repo, false)

	traces := []*londobell.TraceMessage{newTrace(testEpoch, "bafy-hit", atto1e22, "Send", 1, []int64{4}, true)}
	require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, traces, false)))
	require.Empty(t, repo.callLog(), "开关关闭时一个 SQL 都不发")
	require.Empty(t, repo.rowsOf(testEpoch))

	// 回滚同理
	require.NoError(t, task.RollBack(context.Background(), chain.Epoch(testEpoch)))
	require.Empty(t, repo.callLog())
}

func TestExecWriteFailureDoesNotBlockPipeline(t *testing.T) {
	repo := newFakeRepo()
	task := NewLargeTransferTask(repo, true)
	traces := []*londobell.TraceMessage{newTrace(testEpoch, "bafy-hit", atto1e22, "Send", 1, []int64{4}, true)}

	// 该高度的写库失败（PG 超时）⇒ 不返回错误（返回错误会让该高度被判失败并无限重试）
	repo.setErr(fmt.Errorf("write tcp 10.0.0.1:5432: i/o timeout"))
	require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, traces, false)))
	require.Empty(t, repo.rowsOf(testEpoch), "失败时同一事务回滚，不得留下半截数据")
	require.Equal(t, int64(1), task.FailedEpochs(), "失败要留下可观测计数，不能静默")
	require.Zero(t, task.MissingTableWarns(), "网络错误不得被当成表不存在")

	// 后续高度继续正常写（管线没有被打断）
	repo.setErr(nil)
	nextTraces := []*londobell.TraceMessage{newTrace(nextEpoch, "bafy-next", atto1e22, "Send", 1, []int64{8}, true)}
	require.NoError(t, task.Exec(contextWithTraces(t, nextEpoch, nextTraces, false)))
	require.Len(t, repo.rowsOf(nextEpoch), 1)
}

func TestExecMissingTableWarnsOnce(t *testing.T) {
	repo := newFakeRepo()
	task := NewLargeTransferTask(repo, true)
	traces := []*londobell.TraceMessage{newTrace(testEpoch, "bafy-hit", atto1e22, "Send", 1, []int64{4}, true)}

	repo.setErr(fmt.Errorf(`ERROR: relation "chain.large_transfers" does not exist (SQLSTATE 42P01)`))
	for i := 0; i < 3; i++ {
		require.NoError(t, task.Exec(contextWithTraces(t, testEpoch, traces, false)))
	}

	require.Equal(t, int64(3), task.FailedEpochs())
	require.Equal(t, int64(1), task.MissingTableWarns(), "表不存在只 Warn 一次，不刷屏")
}

func TestIsMissingTableErr(t *testing.T) {
	require.False(t, isMissingTableErr(nil))
	require.False(t, isMissingTableErr(fmt.Errorf("write tcp: i/o timeout")))
	require.False(t, isMissingTableErr(fmt.Errorf("ERROR: relation \"chain.large_transfers\" is not owned by current user (SQLSTATE 42501)")))
	require.True(t, isMissingTableErr(fmt.Errorf(`ERROR: relation "chain.large_transfers" does not exist (SQLSTATE 42P01)`)))
	require.True(t, isMissingTableErr(fmt.Errorf("ERROR: relation \"chain.large_transfers\" does not exist (SQLSTATE 42P01)")))
}

func TestRollBack(t *testing.T) {
	t.Run("删除 >= 高度的行，且失败不上抛", func(t *testing.T) {
		repo := newFakeRepo()
		task := NewLargeTransferTask(repo, true)
		for _, epoch := range []int64{testEpoch, nextEpoch, nextEpoch + 1} {
			require.NoError(t, task.Exec(contextWithTraces(t, epoch, []*londobell.TraceMessage{
				newTrace(epoch, fmt.Sprintf("bafy-%d", epoch), atto1e22, "Send", 1, []int64{4}, true),
			}, false)))
		}
		require.Len(t, repo.snapshot(), 3)

		require.NoError(t, task.RollBack(context.Background(), chain.Epoch(nextEpoch)))
		snap := repo.snapshot()
		require.Len(t, snap, 1)
		require.Len(t, snap[testEpoch], 1, "只删 >= 回滚高度")

		repo.setErr(fmt.Errorf("write tcp: i/o timeout"))
		require.NoError(t, task.RollBack(context.Background(), chain.Epoch(testEpoch)),
			"回滚写失败不得上抛（否则同步器卡在回滚-重试循环）")
		require.Equal(t, int64(1), task.FailedEpochs())
	})
}

// ---- 配置开关：默认关闭（老配置文件里没有该字段也不能 panic）----

func TestConfigSwitchDefaultOff(t *testing.T) {
	var nilSyncer *config.Syncer
	require.False(t, nilSyncer.SyncLargeTransfersValue())

	require.False(t, (&config.Syncer{}).SyncLargeTransfersValue(), "未配置 = 关闭")
	require.False(t, (&config.Syncer{SyncLargeTransfers: boolPtr(false)}).SyncLargeTransfersValue())
	require.True(t, (&config.Syncer{SyncLargeTransfers: boolPtr(true)}).SyncLargeTransfersValue())
}

func boolPtr(v bool) *bool { return &v }
