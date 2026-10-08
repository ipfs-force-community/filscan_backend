package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// caliLedgerJSON 是 2026-10-08 Calibnet 实测的 f02 服务流账本形态（impact/README §0）：
//   - 隐式流（共识）评估权重 50%；显式流（服务流）评估权重 45% ⇒ 日程级销毁 = 100−50−45 = 5%；
//   - 份额表 100%：f014260492 73.7463% / f016424204 26.2537%（Writer f011836172）；
//   - 当期 f014260492 已 Claim 10,378.06 FIL；被移除的 stream 3 仍欠 f09346060 4,062.44 FIL（tombstone）。
//
// 说明：accrued / payable 未逐笔公布，取构造的自洽值（accrued = 14,300 FIL），使各受益方待付非负
// 且 Σ(accrued×share/denom) 恰等于 accrued（14,300 × 1e18 = 1.43e22）；账本端点联调后应替换为真实读数。
// 地址用 calibnet 前缀 t0（与节点在该网的返回一致；f/t 前缀由 SmartAddress 原样透传）。
const caliLedgerJSON = `{
  "epoch": 4110339,
  "nv29": true,
  "denom": "1000000000000000000",
  "streams": [
    {"id":1,"implicit":true,"evaluated_weight":"500000000000000000","accrued":"0","claimed_period":"0","payable":"0"},
    {"id":2,"implicit":false,"evaluated_weight":"450000000000000000","accrued":"14300000000000000000000","claimed_period":"10378060000000000000000","payable":"0",
     "recipients":[
       {"address":"t014260492","share":"737463126843657817","payable":"0","claimed_period":"10378060000000000000000"},
       {"address":"t016424204","share":"262536873156342183","payable":"0","claimed_period":"0"}
     ]}
  ],
  "tombstones":[
    {"id":3,"recipients":[{"address":"t09346060","payable":"4062440000000000000000"}]}
  ],
  "liability": "7984380000000000000000"
}`

func mustLedger(t *testing.T, raw string) *londobell.RewardStreamLedger {
	t.Helper()
	var l londobell.RewardStreamLedger
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		t.Fatalf("构造账本样本失败: %s", err)
	}
	return &l
}

type fakeRewardStreamLedgerSource struct {
	ledger *londobell.RewardStreamLedger
	err    error

	calls int
	epoch *chain.Epoch
}

func (f *fakeRewardStreamLedgerSource) GetRewardStreamLedger(_ context.Context, epoch *chain.Epoch) (*londobell.RewardStreamLedger, error) {
	f.calls++
	f.epoch = epoch
	return f.ledger, f.err
}

// NV29 已激活：分账比例＝评估权重百分比（50/45/5）、份额%＝share/denom、待提取＝liability 直通。
func TestRewardStreamLedgerCaliRealSnapshot(t *testing.T) {
	src := &fakeRewardStreamLedgerSource{ledger: mustLedger(t, caliLedgerJSON)}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 4110339}, src)

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if !resp.Nv29 {
		t.Fatal("NV29 已激活，nv29 应为 true")
	}
	if resp.Epoch != 4110339 {
		t.Fatalf("epoch 应取账本高度 4110339，得到 %d", resp.Epoch)
	}

	// 当前分账比例（链上日程的评估权重，一位小数）。
	if resp.CurrentSplit.Miner != "50.0" || resp.CurrentSplit.Service != "45.0" || resp.CurrentSplit.Burn != "5.0" {
		t.Fatalf("current_split 错: got %+v want 50.0/45.0/5.0", resp.CurrentSplit)
	}

	// 待提取总额 = ledger.Liability()；当期已提取合计 = Σ stream.claimed_period。
	if !resp.PendingClaim.Equal(decimal.RequireFromString("7984380000000000000000")) {
		t.Fatalf("pending_claim 应等于 liability，得到 %s", resp.PendingClaim)
	}
	if !resp.ClaimedPeriod.Equal(decimal.RequireFromString("10378060000000000000000")) {
		t.Fatalf("claimed_period 应等于 10,378.06 FIL，得到 %s", resp.ClaimedPeriod)
	}

	// 受益方：按地址升序；share_pct = share/denom（两位小数）；pending 见链上账目计算。
	if len(resp.Recipients) != 3 {
		t.Fatalf("应返回 3 个受益方（2 个 live + 1 个 tombstone），得到 %d", len(resp.Recipients))
	}
	want := []struct {
		addr    string
		share   string
		pending string
	}{
		{"t014260492", "73.75", "167662713864306783100"},
		{"t016424204", "26.25", "3754277286135693216900"},
		{"t09346060", "0.00", "4062440000000000000000"}, // tombstone：无份额、只剩未提
	}
	for i, w := range want {
		got := resp.Recipients[i]
		if got.Address != w.addr || got.SharePct != w.share {
			t.Fatalf("recipients[%d] 地址/份额错: got %s/%s want %s/%s", i, got.Address, got.SharePct, w.addr, w.share)
		}
		if !got.PendingClaim.Equal(decimal.RequireFromString(w.pending)) {
			t.Fatalf("recipients[%d] 待付错: got %s want %s", i, got.PendingClaim, w.pending)
		}
	}

	if src.calls != 1 {
		t.Fatalf("账本取数应只调 1 次，实际 %d", src.calls)
	}
	if src.epoch == nil || src.epoch.Int64() != 4110339 {
		t.Fatalf("取数应带当前链头 epoch，得到 %v", src.epoch)
	}
}

// v18（本网未升 NV29）：nv29=false，金额为 0，受益方空，打 WARN，不报错。
func TestRewardStreamLedgerV18FallsBack(t *testing.T) {
	logs := captureBizLogs(t)
	v18 := &londobell.RewardStreamLedger{Epoch: 6429840, Nv29: false, Denom: decimal.RequireFromString("1000000000000000000")}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 6429840}, &fakeRewardStreamLedgerSource{ledger: v18})

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("v18 不得报错: %s", err)
	}
	if resp.Nv29 {
		t.Fatal("v18 应 nv29=false")
	}
	if !resp.PendingClaim.IsZero() || !resp.ClaimedPeriod.IsZero() {
		t.Fatalf("v18 金额应为 0，得到 pending=%s claimed=%s", resp.PendingClaim, resp.ClaimedPeriod)
	}
	if resp.CurrentSplit == nil || resp.CurrentSplit.Miner != "0.0" || resp.CurrentSplit.Service != "0.0" || resp.CurrentSplit.Burn != "0.0" {
		t.Fatalf("v18 分账比例应为全 0，得到 %+v", resp.CurrentSplit)
	}
	if resp.Recipients == nil || len(resp.Recipients) != 0 {
		t.Fatalf("v18 受益方应为空数组，得到 %#v", resp.Recipients)
	}
	if !strings.Contains(logs.String(), "nv29=false") {
		t.Fatalf("契约要求 v18 也打 WARN，实际日志:\n%s", logs.String())
	}
}

// 取数失败：打 WARN、返回 nv29=false 空账本、err=nil（不得 500）。
func TestRewardStreamLedgerFetchErrorFallsBack(t *testing.T) {
	logs := captureBizLogs(t)
	src := &fakeRewardStreamLedgerSource{err: errors.New("boom")}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 6429840}, src)

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("取数失败不得向上抛错: %s", err)
	}
	if resp.Nv29 || resp.Epoch != 6429840 {
		t.Fatalf("应回退 nv29=false 且 epoch=链头，得到 nv29=%v epoch=%d", resp.Nv29, resp.Epoch)
	}
	if !resp.PendingClaim.IsZero() {
		t.Fatalf("回退金额应为 0，得到 %s", resp.PendingClaim)
	}
	if !strings.Contains(logs.String(), "reward_stream_ledger") || !strings.Contains(logs.String(), "取数失败") {
		t.Fatalf("取数失败必须打 WARN，实际日志:\n%s", logs.String())
	}
}

// 同步器高度取不到：同样回退 nv29=false，不报错、不 500。
func TestRewardStreamLedgerSyncerErrorFallsBack(t *testing.T) {
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{err: errors.New("db down")}, &fakeRewardStreamLedgerSource{})
	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("同步器失败不得向上抛错: %s", err)
	}
	if resp.Nv29 {
		t.Fatal("同步器失败应回退 nv29=false")
	}
}

// 权重之和 > Denom（异常数据）：burn 归零并打 WARN，不出现负百分比。
func TestRewardStreamLedgerBurnClamped(t *testing.T) {
	logs := captureBizLogs(t)
	ledger := &londobell.RewardStreamLedger{
		Epoch: 100, Nv29: true, Denom: decimal.RequireFromString("1000000000000000000"),
		Streams: []*londobell.RewardStreamLedgerStream{
			{ID: 1, Implicit: true, EvaluatedWeight: decimal.RequireFromString("600000000000000000")},
			{ID: 2, Implicit: false, EvaluatedWeight: decimal.RequireFromString("500000000000000000")},
		},
	}
	resp := buildRewardStreamLedgerResponse(ledger, 100)
	if resp.CurrentSplit.Miner != "60.0" || resp.CurrentSplit.Service != "50.0" || resp.CurrentSplit.Burn != "0.0" {
		t.Fatalf("超权重时 burn 应收敛为 0.0，得到 %+v", resp.CurrentSplit)
	}
	if !strings.Contains(logs.String(), "超过 Denom") {
		t.Fatalf("超权重必须打 WARN，实际日志:\n%s", logs.String())
	}
}
