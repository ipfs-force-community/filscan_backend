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

// fakeRewardStreamRecipientSnapshot 假「本周期受益方来源」仓储（快照 + 归集两路），
// 记录调用次数与查询区间供降级/未激活/累计已收 断言。
type fakeRewardStreamRecipientSnapshot struct {
	// 快照来源（按高度）。
	rows  []*po.RewardStreamRecipientEpoch
	err   error
	calls int
	rng   chain.LCRCRange

	// 归集来源（按周期）：本周期行（离场判定 + 累计探针）。
	periodRows  []*po.RewardStreamRecipientPeriod
	periodErr   error
	periodCalls int
	periodStart chain.Epoch
	// 归集来源：按地址批量取（累计 SUM）。
	periodByAddrRows  []*po.RewardStreamRecipientPeriod
	periodByAddrErr   error
	periodByAddrCalls int
	periodByAddrAddrs []string
	// 归集来源：全表 MIN(first_epoch)。
	earliest      chain.Epoch
	earliestFound bool
	earliestErr   error
	earliestCalls int
}

func (f *fakeRewardStreamRecipientSnapshot) ListRewardStreamRecipientsByEpochRange(_ context.Context, epochs chain.LCRCRange) ([]*po.RewardStreamRecipientEpoch, error) {
	f.calls++
	f.rng = epochs
	return f.rows, f.err
}

func (f *fakeRewardStreamRecipientSnapshot) ListRewardStreamRecipientPeriodsByPeriodStart(_ context.Context, periodStart chain.Epoch) ([]*po.RewardStreamRecipientPeriod, error) {
	f.periodCalls++
	f.periodStart = periodStart
	return f.periodRows, f.periodErr
}

func (f *fakeRewardStreamRecipientSnapshot) ListRewardStreamRecipientPeriodsByAddresses(_ context.Context, addresses []string) ([]*po.RewardStreamRecipientPeriod, error) {
	f.periodByAddrCalls++
	f.periodByAddrAddrs = addresses
	return f.periodByAddrRows, f.periodByAddrErr
}

func (f *fakeRewardStreamRecipientSnapshot) EarliestRewardStreamRecipientPeriodEpoch(_ context.Context) (chain.Epoch, bool, error) {
	f.earliestCalls++
	return f.earliest, f.earliestFound, f.earliestErr
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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 4110339}, src, &fakeRewardStreamRecipientSnapshot{}, &fakeRewardStreamRecipientSnapshot{})

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 4139065}, src, &fakeRewardStreamRecipientSnapshot{}, &fakeRewardStreamRecipientSnapshot{})

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 6429840}, &fakeRewardStreamLedgerSource{ledger: v18}, &fakeRewardStreamRecipientSnapshot{}, &fakeRewardStreamRecipientSnapshot{})

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: 6429840}, src, &fakeRewardStreamRecipientSnapshot{}, &fakeRewardStreamRecipientSnapshot{})

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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{err: errors.New("db down")}, &fakeRewardStreamLedgerSource{}, &fakeRewardStreamRecipientSnapshot{}, &fakeRewardStreamRecipientSnapshot{})
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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
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
	biz1 := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: v18}, snap1, snap1)
	biz1.solsticeEpoch = solstice
	if _, err := biz1.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{}); err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap1.calls != 0 {
		t.Fatalf("nv29=false 时不得查快照，实际 %d 次", snap1.calls)
	}

	// 情形二：本网未排期（solsticeEpoch=0）。
	snap2 := &fakeRewardStreamRecipientSnapshot{}
	biz2 := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap2, snap2)
	biz2.solsticeEpoch = 0
	if _, err := biz2.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{}); err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap2.calls != 0 {
		t.Fatalf("未排期时不得查快照，实际 %d 次", snap2.calls)
	}

	// 情形三：当前高度早于激活高度。
	snap3 := &fakeRewardStreamRecipientSnapshot{}
	biz3 := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: solstice - 1}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(solstice - 1)}, snap3, snap3)
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
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
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

// ===== 累计已收（claimed_total / claimed_since_epoch）与「本期离场」改以归集表为准 =====
//
// 归集表 chain.reward_stream_recipient_period 只增不减、跨周期保留，是「累计已收」与「本周期离场」的可靠来源；
// 快照表可能被框架 HistoryClear 修剪，仅作 last_share 补充。以下用例覆盖六条硬要求。

// periodRow 构造一条归集行（Denom=1e18 定点；金额为 attoFIL 十进制字符串）。
func periodRow(addr string, periodStart, firstEpoch, lastEpoch int64, claimedAtto, lastShareAtto string) *po.RewardStreamRecipientPeriod {
	return &po.RewardStreamRecipientPeriod{
		Address:          addr,
		PeriodStartEpoch: periodStart,
		ClaimedInPeriod:  decimal.RequireFromString(claimedAtto),
		LastShare:        decimal.RequireFromString(lastShareAtto),
		FirstEpoch:       firstEpoch,
		LastEpoch:        lastEpoch,
	}
}

// ① claimed_total 为**多周期 SUM**（含离场行）且精度/不变量对得上；claimed_since_epoch = MIN(first_epoch)。
func TestRewardStreamLedgerClaimedTotalSumAcrossPeriods(t *testing.T) {
	const solstice = int64(6_000_000)
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	cur := solstice + 2*periodLen + 5
	start := solstice + 2*periodLen
	prev := start - periodLen

	snap := &fakeRewardStreamRecipientSnapshot{
		// 本周期行：活跃地址 t0199897 + 已离场地址 t0300111（后者不在当前链上状态里）。
		periodRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0199897", start, cur-10, cur-1, "50000000000000000000", "1000000000000000000"),
			periodRow("t0300111", start, cur-9, cur-2, "70000000000000000000", "300000000000000000"),
		},
		// 按地址批量取：含**历史周期**行，验证是跨周期 SUM（不是只看当期）。
		periodByAddrRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0199897", prev, prev+1, prev+9, "100000000000000000000", "1000000000000000000"),
			periodRow("t0199897", start, cur-10, cur-1, "50000000000000000000", "1000000000000000000"),
			periodRow("t0300111", prev, prev+2, prev+8, "70000000000000000000", "300000000000000000"),
		},
		earliest: chain.Epoch(prev + 1), earliestFound: true,
	}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if len(resp.Recipients) != 2 {
		t.Fatalf("应返回 2 行（1 活跃 + 1 离场），得到 %d", len(resp.Recipients))
	}
	// 活跃行：t0199897，累计 = 100e18 + 50e18 = 150e18（跨两个周期求和，含当期）。
	active := resp.Recipients[0]
	if active.Address != "t0199897" || active.Departed {
		t.Fatalf("第 1 行应为活跃 t0199897，得到 %s/departed=%v", active.Address, active.Departed)
	}
	wantActive := decimal.RequireFromString("150000000000000000000")
	if active.ClaimedTotal == nil || !active.ClaimedTotal.Equal(wantActive) {
		t.Fatalf("活跃行 claimed_total 应为 150e18（跨周期 SUM），得到 %v", active.ClaimedTotal)
	}
	// 不变量/精度：SUM 逐位等于两周期之和（decimal 精确，不经 float）。
	if !active.ClaimedTotal.Equal(decimal.RequireFromString("100000000000000000000").Add(decimal.RequireFromString("50000000000000000000"))) {
		t.Fatalf("活跃行 claimed_total 精度不符: %s", active.ClaimedTotal)
	}
	// 离场行：t0300111，累计 = 70e18（**含离场行**）。
	departed := resp.Recipients[1]
	if departed.Address != "t0300111" || !departed.Departed {
		t.Fatalf("第 2 行应为离场 t0300111，得到 %s/departed=%v", departed.Address, departed.Departed)
	}
	if departed.ClaimedTotal == nil || !departed.ClaimedTotal.Equal(decimal.RequireFromString("70000000000000000000")) {
		t.Fatalf("离场行 claimed_total 应为 70e18（含离场行），得到 %v", departed.ClaimedTotal)
	}
	// claimed_since_epoch = MIN(first_epoch) = prev+1。
	if resp.ClaimedSinceEpoch != prev+1 {
		t.Fatalf("claimed_since_epoch 应为 MIN(first_epoch)=%d，得到 %d", prev+1, resp.ClaimedSinceEpoch)
	}
	// 禁 N+1：按地址批量查只调 1 次，且一次带上全部排行地址（活跃 + 离场）。
	if snap.periodByAddrCalls != 1 {
		t.Fatalf("按地址批量查应只调 1 次（禁 N+1），实际 %d", snap.periodByAddrCalls)
	}
	if len(snap.periodByAddrAddrs) != 2 {
		t.Fatalf("批量查地址应为 2 个（活跃+离场），得到 %v", snap.periodByAddrAddrs)
	}
}

// ② 归集表本周期无行（采集件未上生产 / 本周期内还没归集）⇒ 所有行 claimed_total=nil、since=0，JSON 里是 null。
func TestRewardStreamLedgerClaimedTotalNilWhenNoCurrentPeriodRows(t *testing.T) {
	const solstice = int64(6_000_000)
	cur := solstice + 5
	snap := &fakeRewardStreamRecipientSnapshot{
		rows: nil, // 快照表也空
		// 归集表本周期**无行** ⇒ 累计未知。
		periodRows: nil,
		// 即便按地址查能查到历史行，也不该被使用（先探针、后取数）。
		periodByAddrRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0199897", solstice, solstice+1, solstice+9, "100000000000000000000", "1000000000000000000"),
		},
		earliest: chain.Epoch(solstice + 1), earliestFound: true,
	}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if resp.ClaimedSinceEpoch != 0 {
		t.Fatalf("无本周期行时 claimed_since_epoch 应为 0（未知），得到 %d", resp.ClaimedSinceEpoch)
	}
	for _, r := range resp.Recipients {
		if r.ClaimedTotal != nil {
			t.Fatalf("%s 在本周期无归集行时应 claimed_total=nil（未知），得到 %v", r.Address, r.ClaimedTotal)
		}
	}
	// 断言 JSON 里就是 null（不是 0）。
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("序列化响应失败: %s", err)
	}
	if !strings.Contains(string(b), `"claimed_total":null`) {
		t.Fatalf("响应 JSON 应含 \"claimed_total\":null，得到 %s", string(b))
	}
	if snap.periodByAddrCalls != 0 {
		t.Fatalf("探针为空时不应再按地址查（禁臆造 0），实际 %d", snap.periodByAddrCalls)
	}
}

// ③ 有数据（本周期有归集行）但某地址无任何周期行 ⇒ ClaimedTotal=0（非 nil，与「未知」区分）。
func TestRewardStreamLedgerClaimedTotalZeroWhenAddressHasNoRows(t *testing.T) {
	const solstice = int64(6_000_000)
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	cur := solstice + 2*periodLen + 5
	start := solstice + 2*periodLen
	snap := &fakeRewardStreamRecipientSnapshot{
		// 本周期有行（有数据）⇒ 不是「未知」。
		periodRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0199897", start, cur-1, cur-1, "0", "1000000000000000000"),
		},
		// 按地址查只返回**别的**地址的行 ⇒ t0199897 无行 ⇒ 0。
		periodByAddrRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0999999", start, cur-1, cur-1, "123000000000000000000", "0"),
		},
		earliest: chain.Epoch(cur - 1), earliestFound: true,
	}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if len(resp.Recipients) != 1 {
		t.Fatalf("应只返回 1 个活跃受益方，得到 %d", len(resp.Recipients))
	}
	active := resp.Recipients[0]
	if active.ClaimedTotal == nil {
		t.Fatal("有数据时无行的地址应 claimed_total=0（非 nil）")
	}
	if !active.ClaimedTotal.IsZero() {
		t.Fatalf("无行地址 claimed_total 应为 0，得到 %s", active.ClaimedTotal)
	}
	if resp.ClaimedSinceEpoch != cur-1 {
		t.Fatalf("有数据时 claimed_since_epoch 应为 %d，得到 %d", cur-1, resp.ClaimedSinceEpoch)
	}
}

// ④ 快照表完全空（被修剪）时，仅靠归集表也能识别「本期离场」，且 last_share_pct 取自归集表 last_share。
func TestRewardStreamLedgerDepartedFromPeriodOnly(t *testing.T) {
	const solstice = int64(6_000_000)
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	cur := solstice + 2*periodLen + 5
	start := solstice + 2*periodLen
	snap := &fakeRewardStreamRecipientSnapshot{
		rows: nil, // 快照表为空（HistoryClear 已修剪 / 尚未建）
		periodRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0300111", start, cur-30, cur-3, "42000000000000000000", "737500000000000000"),
		},
	}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if snap.calls != 1 {
		t.Fatalf("快照仍应被查 1 次（即便为空），实际 %d", snap.calls)
	}
	if len(resp.Recipients) != 2 {
		t.Fatalf("应返回 2 行（1 活跃 + 1 离场），得到 %d", len(resp.Recipients))
	}
	d := resp.Recipients[1]
	if d.Address != "t0300111" || !d.Departed {
		t.Fatalf("第 2 行应为离场 t0300111，得到 %s/departed=%v", d.Address, d.Departed)
	}
	if d.LastSharePct != "73.75" {
		t.Fatalf("last_share_pct 应取自归集表 last_share（73.75），得到 %s", d.LastSharePct)
	}
	if d.LeftEpoch != cur-3 {
		t.Fatalf("left_epoch 应取归集表 last_epoch=%d，得到 %d", cur-3, d.LeftEpoch)
	}
	if !d.ClaimedPeriod.Equal(decimal.RequireFromString("42000000000000000000")) {
		t.Fatalf("离场行 claimed_period 应取归集表 claimed_in_period=42e18，得到 %s", d.ClaimedPeriod)
	}
	// 纯归集来源无 payable ⇒ 结转记 0，不臆造。
	if !d.PendingClaimCarried.IsZero() || !d.PendingClaim.IsZero() {
		t.Fatalf("纯归集来源离场行 carried/pending 应为 0，得到 %s/%s", d.PendingClaimCarried, d.PendingClaim)
	}
}

// ⑤ 归集表与快照表查询**都报错**时两条路径均降级：接口不报错、无 departed 行、claimed_total 全 nil、since=0，并打 WARN。
func TestRewardStreamLedgerClaimedAndDepartedSourcesErrorDegrade(t *testing.T) {
	const solstice = int64(6_000_000)
	cur := solstice + 5
	logs := captureBizLogs(t)
	snap := &fakeRewardStreamRecipientSnapshot{
		err:             errors.New(`relation "chain.reward_stream_recipient_epoch" does not exist`),
		periodErr:       errors.New(`relation "chain.reward_stream_recipient_period" does not exist`),
		periodByAddrErr: errors.New("boom"),
		earliestErr:     errors.New("boom"),
	}
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: departedActiveLedger(cur)}, snap, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("任一来源失败都不得向上抛错: %s", err)
	}
	if !resp.Nv29 {
		t.Fatal("降级不应影响 nv29（仍按当前链上状态返回）")
	}
	if len(resp.Recipients) != 1 || resp.Recipients[0].Departed {
		t.Fatalf("降级时应只返回当前活跃受益方、无 departed 行，得到 %d 行", len(resp.Recipients))
	}
	for _, r := range resp.Recipients {
		if r.ClaimedTotal != nil {
			t.Fatalf("查询失败时 claimed_total 应全 nil，得到 %v", r.ClaimedTotal)
		}
	}
	if resp.ClaimedSinceEpoch != 0 {
		t.Fatalf("查询失败时 claimed_since_epoch 应为 0，得到 %d", resp.ClaimedSinceEpoch)
	}
	got := logs.String()
	if !strings.Contains(got, "快照失败") || !strings.Contains(got, "归集失败") {
		t.Fatalf("快照/归集两来源失败都应打 WARN，实际日志:\n%s", got)
	}
	if snap.periodByAddrCalls != 0 {
		t.Fatalf("探针（本周期行）已失败，不应再按地址查，实际 %d", snap.periodByAddrCalls)
	}
}

// ⑥ 「累计」≠「当期」：构造既无当期应计又有历史结转的行，验证 claimed_total ≠ claimed_period。
func TestRewardStreamLedgerClaimedTotalDiffersFromCurrentPeriod(t *testing.T) {
	const solstice = int64(4_000_000)
	cur := int64(4139065) // caliZeroShareLedgerJSON 的高度
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	start := periodStartEpoch(solstice, cur, periodLen)
	snap := &fakeRewardStreamRecipientSnapshot{
		periodRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0200442", start, cur-1, cur-1, "5000000000000000000", "0"),
		},
		periodByAddrRows: []*po.RewardStreamRecipientPeriod{
			periodRow("t0200442", start, cur-1, cur-1, "5000000000000000000", "0"),
		},
		earliest: chain.Epoch(cur - 1), earliestFound: true,
	}
	// 这份账本里 t0200442 是「活跃流、份额 0」：当期应计 0、本期已提 0，但历史已提 5 FIL 藏在归集表。
	biz := NewStatisticRewardStreamLedgerBiz(&fakeRewardStreamsSyncer{epoch: cur}, &fakeRewardStreamLedgerSource{ledger: mustLedger(t, caliZeroShareLedgerJSON)}, snap, snap)
	biz.solsticeEpoch = solstice

	resp, err := biz.RewardStreamLedger(context.Background(), filscan.RewardStreamLedgerRequest{})
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	var row *filscan.RewardStreamRecipient
	for _, r := range resp.Recipients {
		if r.Address == "t0200442" {
			row = r
		}
	}
	if row == nil {
		t.Fatal("应返回 t0200442")
	}
	if !row.ClaimedPeriod.IsZero() {
		t.Fatalf("当期已提应为 0，得到 %s", row.ClaimedPeriod)
	}
	if row.ClaimedTotal == nil || !row.ClaimedTotal.Equal(decimal.RequireFromString("5000000000000000000")) {
		t.Fatalf("累计已收应为 5e18，得到 %v", row.ClaimedTotal)
	}
	if row.ClaimedTotal.Equal(row.ClaimedPeriod) {
		t.Fatal("累计(claimed_total) 不应等于当期(claimed_period)")
	}
}
