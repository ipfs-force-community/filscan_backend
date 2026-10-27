package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

// 本文件守护 /api/v1/ChainMessages 的扇出预算：
//
//	线上事故形态：一次地址流水请求对每条流水各发一次 trace_for_message + parent_tipset，
//	50 条消息实测打出 129 次聚合器调用（50 路并发），把聚合器全局分片查询闸门（limit=64）
//	吃光 ⇒ 5s 排队 ⇒ 20~60s 尾延迟 ⇒ 撞客户端 60s 超时（每天 90+ 次 499）。
//
// 断言口径是「下游调用次数」而不是「代码长什么样」：批量化以后，不论地址有多少条流水，
// 一次请求的聚合器调用数必须只与「不同 epoch 数」相关，与消息条数无关。

const (
	imTokenTestAccount      = "f1abcdefghijaccountaaaaaaaaaaaaaaaa11111"
	imTokenTestCounterparty = "f1counterpartybbbbbbbbbbbbbbbbbb22222"
)

type fakeIMTokenAgg struct {
	londobell.Agg

	mu sync.Mutex

	transfers    *londobell.TransferMessagesList
	transfersErr error

	latest []*londobell.Tipset

	traces map[string]*londobell.MessageTrace

	batchErr   error
	batchOmit  map[string]bool
	batchCalls map[int64][]string

	traceCalls  map[string]int
	parentCalls map[int64]int
	parentFees  map[int64]decimal.Decimal
	parentErr   error
}

func newFakeIMTokenAgg() *fakeIMTokenAgg {
	return &fakeIMTokenAgg{
		batchOmit:   map[string]bool{},
		batchCalls:  map[int64][]string{},
		traceCalls:  map[string]int{},
		parentCalls: map[int64]int{},
		parentFees:  map[int64]decimal.Decimal{},
		traces:      map[string]*londobell.MessageTrace{},
	}
}

func (f *fakeIMTokenAgg) LatestTipset(_ context.Context) ([]*londobell.Tipset, error) {
	return f.latest, nil
}

func (f *fakeIMTokenAgg) TransferMessagesByEpoch(_ context.Context, _ chain.SmartAddress, _ types.Filters) (*londobell.TransferMessagesList, error) {
	if f.transfersErr != nil {
		return nil, f.transfersErr
	}
	return f.transfers, nil
}

func (f *fakeIMTokenAgg) BatchTraceForMessage(_ context.Context, start chain.Epoch, cids []string) ([]*londobell.MessageTrace, error) {
	f.mu.Lock()
	f.batchCalls[start.Int64()] = append(f.batchCalls[start.Int64()], cids...)
	err := f.batchErr
	omit := make(map[string]bool, len(f.batchOmit))
	for cid := range f.batchOmit {
		omit[cid] = true
	}
	traces := make(map[string]*londobell.MessageTrace, len(f.traces))
	for cid, trace := range f.traces {
		traces[cid] = trace
	}
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	var out []*londobell.MessageTrace
	for _, cid := range cids {
		if omit[cid] {
			continue
		}
		if trace, ok := traces[cid]; ok {
			out = append(out, trace)
		}
	}
	return out, nil
}

func (f *fakeIMTokenAgg) TraceForMessage(_ context.Context, cid string) ([]*londobell.MessageTrace, error) {
	f.mu.Lock()
	f.traceCalls[cid]++
	trace, ok := f.traces[cid]
	f.mu.Unlock()
	if !ok {
		return nil, nil
	}
	return []*londobell.MessageTrace{trace}, nil
}

func (f *fakeIMTokenAgg) ParentTipset(_ context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	f.mu.Lock()
	f.parentCalls[start.Int64()]++
	err := f.parentErr
	fee, ok := f.parentFees[start.Int64()]
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return []*londobell.ParentTipset{{BaseFee: fee}}, nil
}

type fakeIMTokenAdapter struct {
	londobell.Adapter

	mu     sync.Mutex
	actors map[string]string
	err    error
	calls  int
}

func (f *fakeIMTokenAdapter) Actor(_ context.Context, addr chain.SmartAddress, _ *chain.Epoch) (*londobell.ActorState, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	resolved, ok := f.actors[string(addr)]
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if !ok {
		resolved = addr.Address()
	}
	return &londobell.ActorState{ActorID: string(addr), ActorAddr: resolved, DelegatedAddr: resolved}, nil
}

func (f *fakeIMTokenAdapter) actorCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// imTokenFixture 造一份「2 个 epoch / 3 条不同 cid + 1 条重复 cid + 1 条空 cid」的流水。
func imTokenFixture(t *testing.T) (*IMTokenBiz, *fakeIMTokenAgg, *fakeIMTokenAdapter) {
	t.Helper()
	imTokenResetCaches()
	t.Cleanup(imTokenResetCaches)

	agg := newFakeIMTokenAgg()
	agg.latest = []*londobell.Tipset{{ID: 200}}
	agg.parentFees[100] = decimal.NewFromInt(1000)
	agg.parentFees[101] = decimal.NewFromInt(2000)

	transfer := func(cid string, epoch int64) *londobell.ActorMessages {
		return &londobell.ActorMessages{
			Cid:      cid,
			Epoch:    epoch,
			From:     chain.SmartAddress(imTokenTestAccount),
			To:       chain.SmartAddress(imTokenTestCounterparty),
			Value:    decimal.NewFromInt(1),
			Method:   "Send",
			ExitCode: 0,
		}
	}
	agg.transfers = &londobell.TransferMessagesList{
		TotalCount: 5,
		TransferMessages: []*londobell.ActorMessages{
			transfer("cid-a", 100),
			transfer("cid-b", 100),
			transfer("cid-b", 100), // 重复 cid：只应取一次 trace
			transfer("cid-c", 101),
			transfer("", 101), // 空 cid：旧实现也不取
		},
	}

	trace := func(cid string, epoch int64, nonce int64, to string) *londobell.MessageTrace {
		return &londobell.MessageTrace{
			Cid:        cid,
			Epoch:      chain.Epoch(epoch),
			Nonce:      nonce,
			From:       chain.SmartAddress(imTokenTestCounterparty),
			To:         chain.SmartAddress(to),
			Value:      decimal.NewFromInt(int64(nonce)),
			GasLimit:   1000,
			GasFeeCap:  decimal.NewFromInt(7),
			GasPremium: decimal.NewFromInt(3),
			Method:     "Send",
			GasCost:    &londobell.GasCost{GasUsed: decimal.NewFromInt(500)},
		}
	}
	// cid-a 的收款方是 actor id 形态（"01234"），旧实现会经 adapter.Actor 换成 f 地址
	agg.traces["cid-a"] = trace("cid-a", 100, 5, "01234")
	agg.traces["cid-b"] = trace("cid-b", 100, 7, imTokenTestCounterparty)
	agg.traces["cid-c"] = trace("cid-c", 101, 2, imTokenTestCounterparty)

	adapter := &fakeIMTokenAdapter{actors: map[string]string{"01234": "f01234"}}

	biz := &IMTokenBiz{
		agg:            agg,
		adapter:        adapter,
		AccountAclImpl: acl.NewAccountAclImpl(agg, adapter),
	}
	return biz, agg, adapter
}

func downstreamCalls(agg *fakeIMTokenAgg, adapter *fakeIMTokenAdapter) int {
	agg.mu.Lock()
	defer agg.mu.Unlock()
	total := 1 // transfer_messages_by_epoch（地址流水列表本身，一行）
	total += len(agg.batchCalls)
	for _, n := range agg.traceCalls {
		total += n
	}
	for _, n := range agg.parentCalls {
		total += n
	}
	return total + adapter.actorCalls()
}

func TestChainMessages_BatchesByEpochAndDedupes(t *testing.T) {
	biz, agg, adapter := imTokenFixture(t)

	resp, err := biz.ChainMessages(context.Background(), filscan.ChainMessagesRequest{
		Address: imTokenTestAccount,
	})
	if err != nil {
		t.Fatalf("ChainMessages returned error: %v", err)
	}

	if len(resp.MessageList) != 3 {
		t.Fatalf("want 3 messages, got %d: %+v", len(resp.MessageList), resp.MessageList)
	}
	wantOrder := []string{"cid-c", "cid-b", "cid-a"} // epoch 倒序 → nonce 倒序
	for i, want := range wantOrder {
		if got := resp.MessageList[i].Cid; got != want {
			t.Fatalf("message[%d]: want cid %s, got %s", i, want, got)
		}
	}

	// 批量取数：2 个 epoch ⇒ 2 次 batch；单条 trace_for_message 一次都不该发
	if len(agg.batchCalls) != 2 {
		t.Fatalf("want 2 batch calls (one per epoch), got %d: %+v", len(agg.batchCalls), agg.batchCalls)
	}
	if got := len(agg.batchCalls[100]); got != 2 {
		t.Fatalf("epoch 100 batch should carry 2 unique cids, got %d: %v", got, agg.batchCalls[100])
	}
	if got := len(agg.batchCalls[101]); got != 1 {
		t.Fatalf("epoch 101 batch should carry 1 cid, got %d: %v", got, agg.batchCalls[101])
	}
	if len(agg.traceCalls) != 0 {
		t.Fatalf("batch covered every cid, but per-cid trace was called: %+v", agg.traceCalls)
	}

	// parent_tipset 按 epoch 去重：2 个 epoch ⇒ 2 次（旧实现每条消息 1 次）
	if len(agg.parentCalls) != 2 || agg.parentCalls[100] != 1 || agg.parentCalls[101] != 1 {
		t.Fatalf("want 1 parent_tipset per epoch, got %+v", agg.parentCalls)
	}

	// 扇出预算：整次请求的下游调用数必须与消息条数脱钩
	if calls := downstreamCalls(agg, adapter); calls > 8 {
		t.Fatalf("downstream calls should stay O(epochs), got %d", calls)
	}

	// baseFee 取各自 epoch 的父 tipset
	for _, message := range resp.MessageList {
		want := decimal.NewFromInt(1000)
		if message.Epoch == 101 {
			want = decimal.NewFromInt(2000)
		}
		if !message.BaseFee.Equal(want) {
			t.Fatalf("cid %s: want baseFee %s, got %s", message.Cid, want, message.BaseFee)
		}
	}

	// actor id → f 地址（与旧实现一致）
	for _, message := range resp.MessageList {
		if message.Cid != "cid-a" {
			continue
		}
		if message.To != "f01234" {
			t.Fatalf("actor id 01234 should be resolved to f01234, got %q", message.To)
		}
	}

	// RowId 语义不变：首个为 1，同 epoch 递增，跨 epoch 重置
	if got := resp.MessageList[0].RowId; got != 1 {
		t.Fatalf("message[0].RowId want 1, got %d", got)
	}
	if got := resp.MessageList[1].RowId; got != 1 {
		t.Fatalf("message[1].RowId want 1 (epoch 切换), got %d", got)
	}
	if got := resp.MessageList[2].RowId; got != 2 {
		t.Fatalf("message[2].RowId want 2 (同 epoch 递增), got %d", got)
	}
}

func TestChainMessages_FanoutDoesNotScaleWithMessageCount(t *testing.T) {
	biz, agg, adapter := imTokenFixture(t)

	// 40 条不同消息、3 个 epoch。旧实现是 40 次 trace_for_message + 40 次 parent_tipset
	// （≈ 80+ 次下游调用、50 路并发）；新实现必须只跟 epoch 数有关。
	agg.transfers = &londobell.TransferMessagesList{TotalCount: 40}
	agg.traces = make(map[string]*londobell.MessageTrace, 40)
	for i := 0; i < 40; i++ {
		cid := fmt.Sprintf("cid-%02d", i)
		epoch := int64(300 + i%3)
		agg.transfers.TransferMessages = append(agg.transfers.TransferMessages, &londobell.ActorMessages{
			Cid:    cid,
			Epoch:  epoch,
			From:   chain.SmartAddress(imTokenTestAccount),
			To:     chain.SmartAddress(imTokenTestCounterparty),
			Value:  decimal.NewFromInt(1),
			Method: "Send",
		})
		agg.traces[cid] = &londobell.MessageTrace{
			Cid:     cid,
			Epoch:   chain.Epoch(epoch),
			Nonce:   int64(i),
			From:    chain.SmartAddress(imTokenTestCounterparty),
			To:      chain.SmartAddress(imTokenTestAccount),
			Value:   decimal.NewFromInt(1),
			Method:  "Send",
			GasCost: &londobell.GasCost{GasUsed: decimal.NewFromInt(500)},
		}
	}
	for _, epoch := range []int64{300, 301, 302} {
		agg.parentFees[epoch] = decimal.NewFromInt(epoch)
	}

	resp, err := biz.ChainMessages(context.Background(), filscan.ChainMessagesRequest{Address: imTokenTestAccount})
	if err != nil {
		t.Fatalf("ChainMessages returned error: %v", err)
	}
	if len(resp.MessageList) != 40 {
		t.Fatalf("want 40 messages, got %d", len(resp.MessageList))
	}
	if len(agg.batchCalls) != 3 {
		t.Fatalf("40 条消息只该按 3 个 epoch 分 3 次批量取数，got %+v", agg.batchCalls)
	}
	if len(agg.traceCalls) != 0 {
		t.Fatalf("批量取数覆盖后不该再有单条 trace 调用，got %d 次", len(agg.traceCalls))
	}
	if calls := downstreamCalls(agg, adapter); calls > 10 {
		t.Fatalf("下游调用数必须与消息条数脱钩（40 条 / 3 epoch 应为个位数），got %d", calls)
	}
}

func TestChainMessages_ReusesImmutableDataAcrossPolls(t *testing.T) {
	biz, agg, adapter := imTokenFixture(t)
	req := filscan.ChainMessagesRequest{Address: imTokenTestAccount}

	if _, err := biz.ChainMessages(context.Background(), req); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	first := downstreamCalls(agg, adapter)

	// 客户端按秒轮询同一地址：trace / baseFee / actor 地址都是不可变数据，第二次只该重查
	// 会变的「地址流水列表」本身，不该再打聚合器。
	resp, err := biz.ChainMessages(context.Background(), req)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if len(resp.MessageList) != 3 {
		t.Fatalf("cached poll must return the same 3 messages, got %d", len(resp.MessageList))
	}
	if delta := downstreamCalls(agg, adapter) - first; delta > 1 {
		t.Fatalf("repeat poll should hit immutable caches (delta<=1), got +%d downstream calls", delta)
	}
}

func TestChainMessages_FallsBackToPerCidWhenBatchIncomplete(t *testing.T) {
	biz, agg, _ := imTokenFixture(t)
	agg.batchOmit["cid-b"] = true

	resp, err := biz.ChainMessages(context.Background(), filscan.ChainMessagesRequest{Address: imTokenTestAccount})
	if err != nil {
		t.Fatalf("ChainMessages returned error: %v", err)
	}
	if len(resp.MessageList) != 3 {
		t.Fatalf("want 3 messages after fallback, got %d", len(resp.MessageList))
	}
	if agg.traceCalls["cid-b"] != 1 {
		t.Fatalf("missing cid should be retried once per cid, got %+v", agg.traceCalls)
	}
	if len(agg.traceCalls) != 1 {
		t.Fatalf("only the missing cid may be retried, got %+v", agg.traceCalls)
	}
}

func TestChainMessages_FallsBackWhenBatchUnavailable(t *testing.T) {
	biz, agg, _ := imTokenFixture(t)
	agg.batchErr = errors.New("aggregator: 404 page not found")

	resp, err := biz.ChainMessages(context.Background(), filscan.ChainMessagesRequest{Address: imTokenTestAccount})
	if err != nil {
		t.Fatalf("ChainMessages returned error: %v", err)
	}
	if len(resp.MessageList) != 3 {
		t.Fatalf("want 3 messages after full fallback, got %d", len(resp.MessageList))
	}
	for _, cid := range []string{"cid-a", "cid-b", "cid-c"} {
		if agg.traceCalls[cid] != 1 {
			t.Fatalf("cid %s should be fetched exactly once via fallback, got %+v", cid, agg.traceCalls)
		}
	}
}

func TestChainMessages_RowIdCursorOutOfRangeReturnsEmpty(t *testing.T) {
	biz, _, _ := imTokenFixture(t)

	resp, err := biz.ChainMessages(context.Background(), filscan.ChainMessagesRequest{
		Address: imTokenTestAccount,
		RowId:   99, // 旧实现在这里 slice 越界 panic
	})
	if err != nil {
		t.Fatalf("out-of-range cursor must not error, got %v", err)
	}
	if len(resp.MessageList) != 0 {
		t.Fatalf("out-of-range cursor should return empty page, got %d messages", len(resp.MessageList))
	}
}

func TestChainMessages_PropagatesDownstreamErrors(t *testing.T) {
	biz, agg, adapter := imTokenFixture(t)
	agg.batchErr = errors.New("boom")
	agg.traceCalls = map[string]int{}
	adapter.err = errors.New("adapter down")

	if _, err := biz.ChainMessages(context.Background(), filscan.ChainMessagesRequest{Address: imTokenTestAccount}); err == nil {
		t.Fatal("a failing downstream must fail the request, not return a silently partial list")
	}
}
