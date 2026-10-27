package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/gozelle/mix"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

// ---- fakeIMTokenAgg 的 MessageByEpoch 打桩 ----

func (f *fakeIMTokenAgg) BlockHeader(_ context.Context, filters types.Filters) ([]*londobell.BlockHeader, error) {
	if filters.Start == nil {
		return nil, nil
	}
	return f.blockHeaders[filters.Start.Int64()], nil
}

func (f *fakeIMTokenAgg) MessagesForBlock(_ context.Context, blockCid string, _ types.Filters) ([]*londobell.MessageTrace, error) {
	return f.blockMessages[blockCid], nil
}

func epochTrace(cid string, epoch int64) *londobell.MessageTrace {
	return &londobell.MessageTrace{
		Cid:        cid,
		Epoch:      chain.Epoch(epoch),
		Nonce:      1,
		From:       chain.SmartAddress(imTokenTestCounterparty),
		To:         chain.SmartAddress(imTokenTestAccount),
		Value:      decimal.NewFromInt(1),
		GasLimit:   1000,
		GasFeeCap:  decimal.NewFromInt(2),
		GasPremium: decimal.NewFromInt(1),
		Method:     "Send",
		ExitCode:   0,
	}
}

// TestMessageByEpoch_FutureEpochIsClientError 客户端拿 head+1 轮询是常态（线上每天 3000+ 次），
// 「问得太早」必须按客户端错误返回，不能记成服务端 500。
func TestMessageByEpoch_FutureEpochIsClientError(t *testing.T) {
	biz, _, _ := imTokenFixture(t) // latest = 200

	resp, err := biz.MessageByEpoch(context.Background(), filscan.MessageListRequest{Epoch: 250})
	if err == nil {
		t.Fatalf("超前 epoch 应报错，实际返回 %+v", resp)
	}
	var warn *mix.Warn
	if !errors.As(err, &warn) {
		t.Fatalf("期望客户端错误类型 *mix.Warn，得到 %T: %v", err, err)
	}
	if warn.Code != 400 {
		t.Fatalf("错误码 = %d, want 400", warn.Code)
	}
}

// TestMessageByEpoch_BatchesTracesInsteadOfPerCid 一个 epoch 的消息全在同一 epoch 里，
// 应一次批量取 trace（旧实现每条消息一次 TraceForMessage，50 条就把聚合器闸门吃光）。
func TestMessageByEpoch_BatchesTracesInsteadOfPerCid(t *testing.T) {
	biz, agg, _ := imTokenFixture(t) // latest = 200

	agg.blockHeaders[150] = []*londobell.BlockHeader{{ID: "blk-150", Epoch: 150}}
	agg.parentFees[151] = decimal.NewFromInt(5)
	agg.blockMessages["blk-150"] = []*londobell.MessageTrace{
		{Cid: "c1", Epoch: 150, Method: "Send"},
		{Cid: "c2", Epoch: 150, Method: "Send"},
		{Cid: "c1", Epoch: 150, Method: "Send"}, // 重复 cid：只取一次
		{Cid: "c3", Epoch: 150, Method: "Send"},
		{Cid: "c4", Epoch: 150, Method: "Other"}, // 被类型过滤掉
	}
	for _, cid := range []string{"c1", "c2", "c3"} {
		agg.traces[cid] = epochTrace(cid, 150)
	}

	resp, err := biz.MessageByEpoch(context.Background(), filscan.MessageListRequest{Epoch: 150})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(resp.MessageList); got != 3 {
		t.Fatalf("返回消息数 = %d, want 3", got)
	}

	agg.mu.Lock()
	batchCalls := len(agg.batchCalls[150])
	traceCalls := len(agg.traceCalls)
	agg.mu.Unlock()
	if batchCalls != 3 {
		t.Fatalf("批量 trace 覆盖 cid 数 = %d, want 3（c1/c2/c3）", batchCalls)
	}
	if traceCalls != 0 {
		t.Fatalf("不应再逐条 TraceForMessage，实际逐条调用 %d 个 cid", traceCalls)
	}
}
