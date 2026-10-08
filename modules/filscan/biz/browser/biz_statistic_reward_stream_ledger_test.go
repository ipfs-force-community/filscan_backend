package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// caliLedgerJSON 是 2026-10-08 Calibnet 实测的 f02 服务流账本形态（impact/README §0）：
//   - 隐式流（共识）评估权重 50%；显式流（服务流）评估权重 45% ⇒ 日程级销毁 = 100−50−45 = 5%；
//   - 份额表 100%：f0199897 73.7463% / f0200442 26.2537%（Writer f0200116）；
//   - 当期 f0199897 已 Claim 10,378.06 FIL；被移除的 stream 3 仍欠 f0200206 4,062.44 FIL（tombstone）。
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
       {"address":"t0199897","share":"737463126843657817","payable":"0","claimed_period":"10378060000000000000000"},
       {"address":"t0200442","share":"262536873156342183","payable":"0","claimed_period":"0"}
     ]}
  ],
  "tombstones":[
    {"id":3,"recipients":[{"address":"t0200206","payable":"4062440000000000000000"}]}
  ],
  "liability": "7984380000000000000000"
}`

// fakeRewardStreamRecipientSnapshot 假「本周期受益方快照」仓储（窄接口），记录调用次数与查询区间供降级/未激活断言。
type fakeRewardStreamRecipientSnapshot struct {
	rows  []*po.RewardStreamRecipientEpoch
	err   error
	calls int
	rng   chain.LCRCRange
}

func (f *fakeRewardStreamRecipientSnapshot) ListRewardStreamRecipientsByEpochRange(_ context.Context, epochs chain.LCRCRange) ([]*po.RewardStreamRecipientEpoch, error) {
	f.calls++
	f.rng = epochs
	return f.rows, f.err
}

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 4110339}, src, &fakeRewardStreamRecipientSnapshot{})

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

	// 受益方：按待付 pending_claim 降序（pending 相等按地址升序）；share_pct = share/denom（两位小数）；
	// claimed_period = 各显式流 recipient 级 claimed_period 之和（tombstone 无该字段，计 0）。
	if len(resp.Recipients) != 3 {
		t.Fatalf("应返回 3 个受益方（2 个 live + 1 个 tombstone），得到 %d", len(resp.Recipients))
	}
	want := []struct {
		addr    string
		share   string
		pending string
		current string
		carried string
		claimed string
		removed bool
		zero    bool
	}{
		// 排行顺序＝待付降序：t0200206(4062.44) > t0200442(3754.28) > t0199897(167.66)。
		// 注意 t0199897 份额最大却排最后：其本期已提取 10,378.06 FIL（claimed_period），待付被扣减 ——
		// 这正是新增「已付」列要暴露的信息（金额取链上实测的 recipient 级 ClaimedPeriod；accrued/payable 为构造自洽值，见上）。
		// removed_stream：只有 t0200206 是「只出现在已移除流（tombstone）里、当前份额为 0」的遗留欠款收款人 ⇒ true。
		// 拆列口径：pending_claim_current = max(0, 本期应计 − 本期已提)；carried = 总额 − current。
		// t0200206 是纯结转（tombstone 只有 payable）⇒ current=0、carried=全额；
		// t0199897 本期应计 10,545.72 FIL、本期已提 10,378.06 FIL ⇒ current 只剩 167.66 FIL；
		// t0200442 本期应计 3,754.28 FIL、未提取 ⇒ 全部落在当期列。
		{"t0200206", "0.00", "4062440000000000000000", "0", "4062440000000000000000", "0", true, false},
		{"t0200442", "26.25", "3754277286135693216900", "3754277286135693216900", "0", "0", false, false},
		{"t0199897", "73.75", "167662713864306783100", "167662713864306783100", "0", "10378060000000000000000", false, false},
	}
	for i, w := range want {
		got := resp.Recipients[i]
		if got.Address != w.addr || got.SharePct != w.share {
			t.Fatalf("recipients[%d] 地址/份额错: got %s/%s want %s/%s", i, got.Address, got.SharePct, w.addr, w.share)
		}
		if !got.PendingClaim.Equal(decimal.RequireFromString(w.pending)) {
			t.Fatalf("recipients[%d] 待付错: got %s want %s", i, got.PendingClaim, w.pending)
		}
		if !got.ClaimedPeriod.Equal(decimal.RequireFromString(w.claimed)) {
			t.Fatalf("recipients[%d] 已付 claimed_period 错: got %s want %s", i, got.ClaimedPeriod, w.claimed)
		}
		if !got.PendingClaimCurrent.Equal(decimal.RequireFromString(w.current)) {
			t.Fatalf("recipients[%d] 当期应收错: got %s want %s", i, got.PendingClaimCurrent, w.current)
		}
		if !got.PendingClaimCarried.Equal(decimal.RequireFromString(w.carried)) {
			t.Fatalf("recipients[%d] 跨周期应收错: got %s want %s", i, got.PendingClaimCarried, w.carried)
		}
		// 不变量：当期 + 跨周期 == 总额（显示约定，任何一行都不许破）
		if !got.PendingClaimCurrent.Add(got.PendingClaimCarried).Equal(got.PendingClaim) {
			t.Fatalf("recipients[%d] 拆列不守恒: current %s + carried %s != pending %s",
				i, got.PendingClaimCurrent, got.PendingClaimCarried, got.PendingClaim)
		}
		if got.RemovedStream != w.removed {
			t.Fatalf("recipients[%d] removed_stream 错: got %v want %v", i, got.RemovedStream, w.removed)
		}
		if got.ZeroShare != w.zero {
			t.Fatalf("recipients[%d] zero_share 错: got %v want %v", i, got.ZeroShare, w.zero)
		}
	}
	// 至少一条 recipient 的已付非零，确保「已付」列真的被测到（不是全 0 的空断言）。
	nonZeroClaimed := false
	for _, r := range resp.Recipients {
		if r.ClaimedPeriod.IsPositive() {
			nonZeroClaimed = true
			break
		}
	}
	if !nonZeroClaimed {
		t.Fatal("样本里应至少有一条受益方 claimed_period 非零，否则「已付」列未被真正覆盖")
	}

	if src.calls != 1 {
		t.Fatalf("账本取数应只调 1 次，实际 %d", src.calls)
	}
	if src.epoch == nil || src.epoch.Int64() != 4110339 {
		t.Fatalf("取数应带当前链头 epoch，得到 %v", src.epoch)
	}
}

// caliZeroShareLedgerJSON 是 2026-10-09 Calibnet 实测的 f02 账本（epoch 4139065）：
//   - 活跃流只剩 2 条：隐式（矿工）50% + 显式服务流 45%，后者的**唯一受益方 t0200442 份额就是 0**
//     （链上确实存在「流还在、份额被置 0」的收款人：epoch 4138800 起 share 由 26.25% 变 0，应得转成 payable）；
//   - tombstone 里只剩 t0200206 的 4,062.44 FIL；
//   - liability 11,570.215563980994166781 = 两行欠款之和（逐位相等）。
//
// 这条样本用来锁 zero_share：份额 0% 但**不**是「已移除流」的行也必须被标记，否则页面上同为 0% 的两行一个带说明一个不带。
const caliZeroShareLedgerJSON = `{
  "epoch": 4139065,
  "nv29": true,
  "denom": "1000000000000000000",
  "streams": [
    {"id":1,"implicit":true,"evaluated_weight":"500000000000000000","accrued":"0","claimed_period":"0","payable":"0"},
    {"id":2,"implicit":false,"evaluated_weight":"450000000000000000","accrued":"0","claimed_period":"0","payable":"7507775215796681617728",
     "recipients":[{"address":"t0200442","share":"0","payable":"7507775215796681617728","claimed_period":"0"}]}
  ],
  "tombstones":[{"id":3,"recipients":[{"address":"t0200206","payable":"4062440348184312549053"}]}],
  "liability":"11570215563980994166781"
}`

// 活跃流里份额为 0 的收款人：zero_share=true、removed_stream=false（与 tombstone 行区分）；
// 两行都是 0.00% 但来源不同，前端据此各给一句说明。
func TestRewardStreamLedgerZeroShareLiveStream(t *testing.T) {
	src := &fakeRewardStreamLedgerSource{ledger: mustLedger(t, caliZeroShareLedgerJSON)}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 4139065}, src, &fakeRewardStreamRecipientSnapshot{})

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if !resp.Nv29 {
		t.Fatal("NV29 已激活，nv29 应为 true")
	}
	if resp.CurrentSplit.Miner != "50.0" || resp.CurrentSplit.Service != "45.0" || resp.CurrentSplit.Burn != "5.0" {
		t.Fatalf("current_split 错: got %+v", resp.CurrentSplit)
	}
	// 代付总额必须等于 actor 自己的 liability（逐位相等）。
	if !resp.PendingClaim.Equal(decimal.RequireFromString("11570215563980994166781")) {
		t.Fatalf("pending_claim 应等于 liability，得到 %s", resp.PendingClaim)
	}
	if len(resp.Recipients) != 2 {
		t.Fatalf("应返回 2 个受益方，得到 %d", len(resp.Recipients))
	}
	first, second := resp.Recipients[0], resp.Recipients[1]
	// 排序＝待付降序：t0200442(7507.78) > t0200206(4062.44)
	if first.Address != "t0200442" || first.SharePct != "0.00" || first.RemovedStream || !first.ZeroShare {
		t.Fatalf("活跃流份额 0 的行应为 t0200442 / 0.00%% / removed=false / zero=true，得到 %s/%s/removed=%v/zero=%v",
			first.Address, first.SharePct, first.RemovedStream, first.ZeroShare)
	}
	if !first.PendingClaim.Equal(decimal.RequireFromString("7507775215796681617728")) {
		t.Fatalf("t0200442 待付应等于 payable 7,507.78 FIL，得到 %s", first.PendingClaim)
	}
	// 份额 0 ⇒ 本期应计 0 ⇒ 当期应收 0；这 7,507.78 FIL 全部是跨周期结转（链上 payable）。
	if !first.PendingClaimCurrent.IsZero() {
		t.Fatalf("t0200442 当期应收应为 0（本期应计为 0），得到 %s", first.PendingClaimCurrent)
	}
	if !first.PendingClaimCarried.Equal(decimal.RequireFromString("7507775215796681617728")) {
		t.Fatalf("t0200442 跨周期应收应等于 payable，得到 %s", first.PendingClaimCarried)
	}
	if second.Address != "t0200206" || second.SharePct != "0.00" || !second.RemovedStream || second.ZeroShare {
		t.Fatalf("tombstone 行应为 t0200206 / 0.00%% / removed=true / zero=false，得到 %s/%s/removed=%v/zero=%v",
			second.Address, second.SharePct, second.RemovedStream, second.ZeroShare)
	}
	// 两行都是 0.00%，但必须恰好一行 removed、一行 zero（否则页面上又会出现「一个有一个没有」）。
	if first.ZeroShare == second.ZeroShare {
		t.Fatal("两行应一个 removed_stream 一个 zero_share，出现「同为 0% 却标记不一致」")
	}
}

// v18（本网未升 NV29）：nv29=false，金额为 0，受益方空，打 WARN，不报错。
func TestRewardStreamLedgerV18FallsBack(t *testing.T) {
	logs := captureBizLogs(t)
	v18 := &londobell.RewardStreamLedger{Epoch: 6429840, Nv29: false, Denom: decimal.RequireFromString("1000000000000000000")}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 6429840}, &fakeRewardStreamLedgerSource{ledger: v18}, &fakeRewardStreamRecipientSnapshot{})

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 6429840}, src, &fakeRewardStreamRecipientSnapshot{})

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{err: errors.New("db down")}, &fakeRewardStreamLedgerSource{}, &fakeRewardStreamRecipientSnapshot{})
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

// departedActiveLedger 构造一份 nv29=true 的当前账本：一条显式流、一个活跃受益方 t0199897（份额 100%、本期应计 1e18）。
// 用于演练「本周期离场者」补齐路径（活跃行待付 1e18）。
func departedActiveLedger(epoch int64) *londobell.RewardStreamLedger {
	return &londobell.RewardStreamLedger{
		Epoch: epoch, Nv29: true, Denom: decimal.RequireFromString("1000000000000000000"),
		Streams: []*londobell.RewardStreamLedgerStream{
			{ID: 1, Implicit: true, EvaluatedWeight: decimal.RequireFromString("500000000000000000")},
			{ID: 2, Implicit: false, EvaluatedWeight: decimal.RequireFromString("450000000000000000"),
				Accrued: decimal.RequireFromString("1000000000000000000"),
				Recipients: []*londobell.RewardStreamLedgerRecipient{
					{Address: "t0199897", Share: decimal.RequireFromString("1000000000000000000"), Payable: decimal.Zero, ClaimedPeriod: decimal.Zero},
				}},
		},
		Liability: decimal.RequireFromString("1000000000000000000"),
	}
}

// ①有离场地址：补在末尾、五个金额/份额字段口径正确、不变量 pending==current+carried 成立；
// 周期起点按 nv29Epoch + ((cur−nv29Epoch)/periodLen)*periodLen 计算，且查询区间为 [周期起点, 当前高度]。
func TestRewardStreamLedgerDepartedAppended(t *testing.T) {
	const solstice = int64(6_000_000)
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	cur := solstice + 2*periodLen + 5
	wantStart := solstice + 2*periodLen

	snap := &fakeRewardStreamRecipientSnapshot{rows: []*po.RewardStreamRecipientEpoch{
		// 活跃地址（当前仍在）——不得当作离场者补入。
		{Epoch: cur - 100, Address: "t0199897", Share: decimal.RequireFromString("1000000000000000000"), Payable: decimal.RequireFromString("5000000000000000000")},
		// 离场者两行：口径取本周期内 epoch 最大那一行。
		{Epoch: cur - 900, Address: "t0300111", Share: decimal.RequireFromString("300000000000000000"), Payable: decimal.RequireFromString("111000000000000000000")},
		{Epoch: cur - 50, Address: "t0300111", Share: decimal.RequireFromString("737500000000000000"), Payable: decimal.RequireFromString("222000000000000000000"), ClaimedPeriod: decimal.RequireFromString("42000000000000000000")},
	}}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap.calls != 1 {
		t.Fatalf("nv29 已激活且已排期时应查 1 次快照，实际 %d", snap.calls)
	}
	if snap.rng.GteBegin.Int64() != wantStart || snap.rng.LteEnd.Int64() != cur {
		t.Fatalf("快照查询区间应为 [周期起点 %d, 当前高度 %d]，得到 [%d, %d]",
			wantStart, cur, snap.rng.GteBegin.Int64(), snap.rng.LteEnd.Int64())
	}
	if len(resp.Recipients) != 2 {
		t.Fatalf("应返回 2 个受益方（1 活跃 + 1 离场），得到 %d", len(resp.Recipients))
	}
	a := resp.Recipients[0]
	if a.Address != "t0199897" || a.Departed {
		t.Fatalf("活跃行应为 t0199897/departed=false，得到 %s/departed=%v", a.Address, a.Departed)
	}
	d := resp.Recipients[1]
	if d.Address != "t0300111" || !d.Departed {
		t.Fatalf("离场行应为 t0300111/departed=true，得到 %s/departed=%v", d.Address, d.Departed)
	}
	if d.SharePct != "0.00" {
		t.Fatalf("离场行当前无份额 ⇒ share_pct 应 0.00，得到 %s", d.SharePct)
	}
	if d.LastSharePct != "73.75" {
		t.Fatalf("离场行 last_share_pct 应为离场行 Share 折算 73.75，得到 %s", d.LastSharePct)
	}
	if d.LeftEpoch != cur-50 {
		t.Fatalf("离场行 left_epoch 应取本周期内 epoch 最大行 %d，得到 %d", cur-50, d.LeftEpoch)
	}
	if !d.PendingClaimCurrent.IsZero() {
		t.Fatalf("离场行当期应收应为 0，得到 %s", d.PendingClaimCurrent)
	}
	if !d.PendingClaimCarried.Equal(decimal.RequireFromString("222000000000000000000")) {
		t.Fatalf("离场行跨周期应收应取离场行 Payable 222e18，得到 %s", d.PendingClaimCarried)
	}
	if !d.PendingClaim.Equal(d.PendingClaimCarried) {
		t.Fatalf("离场行 pending_claim 应等于 carried，得到 %s vs %s", d.PendingClaim, d.PendingClaimCarried)
	}
	if !d.ClaimedPeriod.Equal(decimal.RequireFromString("42000000000000000000")) {
		t.Fatalf("离场行 claimed_period 应取离场行值 42e18，得到 %s", d.ClaimedPeriod)
	}
	if !d.PendingClaimCurrent.Add(d.PendingClaimCarried).Equal(d.PendingClaim) {
		t.Fatalf("离场行拆列不守恒: current %s + carried %s != pending %s", d.PendingClaimCurrent, d.PendingClaimCarried, d.PendingClaim)
	}
}

// ②快照查询报错（如表未建）时降级：接口正常返回、无 departed 行、无 error、打 WARN。
func TestRewardStreamLedgerDepartedSnapshotErrorDegrades(t *testing.T) {
	const solstice = int64(6_000_000)
	cur := solstice + 5
	logs := captureBizLogs(t)
	snap := &fakeRewardStreamRecipientSnapshot{err: errors.New(`relation "chain.reward_stream_recipient_epoch" does not exist`)}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("快照表不存在时不得向上抛错: %s", err)
	}
	if !resp.Nv29 {
		t.Fatal("快照失败不应影响 nv29（仍应 true，按只有当前状态返回）")
	}
	if snap.calls != 1 {
		t.Fatalf("应尝试查 1 次快照，实际 %d", snap.calls)
	}
	if len(resp.Recipients) != 1 || resp.Recipients[0].Departed {
		t.Fatalf("降级时应只返回当前活跃受益方、无 departed 行，得到 %d 行", len(resp.Recipients))
	}
	if !strings.Contains(logs.String(), "快照失败") {
		t.Fatalf("快照取数失败必须打 WARN，实际日志:\n%s", logs.String())
	}
}

// ③nv29 未激活时不查快照（假实现计数断言 0 次）：nv29=false / 未排期(solsticeEpoch=0) / 当前高度早于激活高度。
func TestRewardStreamLedgerDepartedNotQueriedWhenInactive(t *testing.T) {
	const solstice = int64(6_000_000)
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	cur := solstice + periodLen + 1

	// 情形一：账本 nv29=false（本网未激活）。
	v18 := &londobell.RewardStreamLedger{Epoch: cur, Nv29: false, Denom: decimal.RequireFromString("1000000000000000000")}
	snap1 := &fakeRewardStreamRecipientSnapshot{}
	biz1 := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: v18}, snap1)
	biz1.solsticeEpoch = solstice
	if _, err := biz1.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{}); err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap1.calls != 0 {
		t.Fatalf("nv29=false 时不得查快照，实际 %d 次", snap1.calls)
	}

	// 情形二：本网未排期（solsticeEpoch=0）。
	snap2 := &fakeRewardStreamRecipientSnapshot{}
	biz2 := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap2)
	biz2.solsticeEpoch = 0
	if _, err := biz2.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{}); err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap2.calls != 0 {
		t.Fatalf("未排期时不得查快照，实际 %d 次", snap2.calls)
	}

	// 情形三：当前高度早于激活高度。
	snap3 := &fakeRewardStreamRecipientSnapshot{}
	biz3 := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: solstice - 1}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(solstice - 1)}, snap3)
	biz3.solsticeEpoch = solstice
	if _, err := biz3.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{}); err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap3.calls != 0 {
		t.Fatalf("当前高度早于激活高度时不得查快照，实际 %d 次", snap3.calls)
	}
}

// ④两个（含并列）离场地址按 carried 降序（相等按地址升序），且一律排在活跃行之后。
func TestRewardStreamLedgerDepartedOrderByCarriedDesc(t *testing.T) {
	const solstice = int64(6_000_000)
	cur := solstice + 3
	snap := &fakeRewardStreamRecipientSnapshot{rows: []*po.RewardStreamRecipientEpoch{
		{Epoch: cur - 3, Address: "t0300202", Share: decimal.RequireFromString("100000000000000000"), Payable: decimal.RequireFromString("500000000000000000000")},
		{Epoch: cur - 2, Address: "t0300201", Share: decimal.RequireFromString("200000000000000000"), Payable: decimal.RequireFromString("900000000000000000000")},
		{Epoch: cur - 1, Address: "t0300200", Share: decimal.RequireFromString("300000000000000000"), Payable: decimal.RequireFromString("500000000000000000000")},
	}}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if len(resp.Recipients) != 4 { // 1 活跃 + 3 离场
		t.Fatalf("应返回 4 个受益方，得到 %d", len(resp.Recipients))
	}
	if resp.Recipients[0].Departed {
		t.Fatal("活跃行应排在所有离场行之前")
	}
	got := make([]string, 0, 3)
	for _, r := range resp.Recipients[1:] {
		got = append(got, r.Address)
	}
	// carried 降序：t0300201(900) 在前；两个 500 相等 ⇒ 按地址升序 t0300200 < t0300202。
	want := []string{"t0300201", "t0300200", "t0300202"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("离场行顺序应 %v，得到 %v", want, got)
	}
}
