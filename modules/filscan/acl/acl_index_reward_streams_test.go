package acl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// nv29Streams 两行 NV29(Solstice) 后的真实形态：TotalStoragePowerReward=0，
// 只有 TotalMintedReward/TotalBurnMinted/TotalExplicitMinted 有值。
//
//	miner   = Δ(minted − burn − explicit) = (1100e18 − 700e18) = 400e18
//	service = Δexplicit                   = (340e18 − 200e18)  = 140e18
//	burn    = Δburn                       = (160e18 − 100e18)  = 60e18
//	total   = Δminted                     = (1600e18 − 1000e18) = 600e18
//
// 且满足 miner = total − service − burn。
func nv29Streams() []*londobell.RewardStream {
	return []*londobell.RewardStream{
		{
			Epoch:               1000,
			TotalMintedReward:   decimal.RequireFromString("1000000000000000000000"),
			TotalBurnMinted:     decimal.RequireFromString("100000000000000000000"),
			TotalExplicitMinted: decimal.RequireFromString("200000000000000000000"),
		},
		{
			Epoch:               1120,
			TotalMintedReward:   decimal.RequireFromString("1600000000000000000000"),
			TotalBurnMinted:     decimal.RequireFromString("160000000000000000000"),
			TotalExplicitMinted: decimal.RequireFromString("340000000000000000000"),
		},
	}
}

// ① 三流差分正确。
func TestRewardStreamDeltas24HCorrect(t *testing.T) {
	d := rewardStreamDeltas24H(nv29Streams())
	if !d.OK {
		t.Fatal("两行 NV29 快照应 OK=true")
	}
	if !d.Miner.Equal(decimal.RequireFromString("400000000000000000000")) {
		t.Fatalf("miner 差分错: got %s want 400e18", d.Miner)
	}
	if !d.Service.Equal(decimal.RequireFromString("140000000000000000000")) {
		t.Fatalf("service 差分错: got %s want 140e18", d.Service)
	}
	if !d.Burn.Equal(decimal.RequireFromString("60000000000000000000")) {
		t.Fatalf("burn 差分错: got %s want 60e18", d.Burn)
	}
	if !d.Total.Equal(decimal.RequireFromString("600000000000000000000")) {
		t.Fatalf("total 差分错: got %s want 600e18", d.Total)
	}
}

// ② 恒等式：miner = total − service − burn。
func TestRewardStreamDeltas24HInvariant(t *testing.T) {
	d := rewardStreamDeltas24H(nv29Streams())
	if !d.OK {
		t.Fatal("应 OK=true")
	}
	want := d.Total.Sub(d.Service).Sub(d.Burn)
	if !d.Miner.Equal(want) {
		t.Fatalf("miner 必须等于 total−service−burn: miner=%s want=%s", d.Miner, want)
	}
}

// ③ 首尾缺失（不足两行）⇒ OK=false 且四者为零值（调用方据此置 0）。
func TestGetRewardStreamDeltas24HInsufficient(t *testing.T) {
	agg := &fakeIndexAgg{streams: nv29Streams()[:1]}
	a := newTestAcl(agg, &fakeIndexAdapter{}, &fakeWinCountRepo{})

	d, err := a.GetRewardStreamDeltas24H(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("取数成功（仅快照不足）不应报错: %s", err)
	}
	if d.OK {
		t.Fatal("单行快照应 OK=false")
	}
	if !d.Miner.IsZero() || !d.Service.IsZero() || !d.Burn.IsZero() || !d.Total.IsZero() {
		t.Fatalf("OK=false 时四个金额必须为零值: %+v", d)
	}
}

// ④ 首尾同一高度 ⇒ OK=false。
func TestRewardStreamDeltas24HSameEpoch(t *testing.T) {
	rows := nv29Streams()
	rows[1].Epoch = rows[0].Epoch
	if d := rewardStreamDeltas24H(rows); d.OK {
		t.Fatal("首尾同高应 OK=false")
	}

	agg := &fakeIndexAgg{streams: rows}
	a := newTestAcl(agg, &fakeIndexAdapter{}, &fakeWinCountRepo{})
	d, err := a.GetRewardStreamDeltas24H(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if d.OK {
		t.Fatal("首尾同高经 GetRewardStreamDeltas24H 也应 OK=false")
	}
}

// ⑤ 上游报错 ⇒ 返回 err、OK=false、不 panic。
func TestGetRewardStreamDeltas24HAggErrorNoPanic(t *testing.T) {
	agg := &fakeIndexAgg{err: errors.New("dial tcp: connection refused")}
	a := newTestAcl(agg, &fakeIndexAdapter{}, &fakeWinCountRepo{})

	d, err := a.GetRewardStreamDeltas24H(context.Background(), 6429840)
	if err == nil {
		t.Fatal("上游报错应返回 err")
	}
	if d.OK {
		t.Fatal("上游报错应 OK=false")
	}
}

// 硬要求：首页路径对 aggregator 的 reward_streams 调用次数必须为 1，且三流与赢票奖励同源。
func TestGetHomeRewardStreams24HSingleAggCall(t *testing.T) {
	agg := &fakeIndexAgg{streams: nv29Streams()}
	adapter := &fakeIndexAdapter{err: errors.New("正常路径不该取旧口径 actor")}
	repo := &fakeWinCountRepo{sum: 100, covered: 2880}
	a := newTestAcl(agg, adapter, repo)

	const epoch chain.Epoch = 6429840
	hs, err := a.GetHomeRewardStreams24H(context.Background(), epoch)
	wc, d := hs.WinCountReward, hs.Deltas
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if agg.calls != 1 {
		t.Fatalf("首页一次请求对 reward_streams 必须只调 1 次，实际 %d 次", agg.calls)
	}
	if agg.start != epoch-2880 || agg.end != epoch+1 {
		t.Fatalf("窗口错: got [%d,%d) want [%d,%d)", agg.start, agg.end, epoch-2880, epoch+1)
	}
	// 400e18 atto / 100 = 4e18 atto。
	if !wc.Equal(decimal.RequireFromString("4000000000000000000")) {
		t.Fatalf("每赢票奖励错: got %s want 4e18", wc)
	}
	if !d.OK || !d.Miner.Equal(decimal.RequireFromString("400000000000000000000")) {
		t.Fatalf("三流明细应来自同一次取数: %+v", d)
	}
	if adapter.calls != 0 {
		t.Fatalf("正常路径不该回退旧口径（actor 调用 %d 次）", adapter.calls)
	}
}

// 首页路径上游报错：赢票奖励回退旧口径（WARN）、三流 OK=false、仍只调 1 次、不 panic。
func TestGetHomeRewardStreams24HAggErrorFallsBack(t *testing.T) {
	logs := captureAclLogs(t)
	agg := &fakeIndexAgg{err: errors.New("agg down")}
	adapter := &fakeIndexAdapter{state: map[string]interface{}{"ThisEpochReward": legacyThisEpochReward}}
	repo := &fakeWinCountRepo{sum: 1000, covered: 2880}
	a := newTestAcl(agg, adapter, repo)

	hs, err := a.GetHomeRewardStreams24H(context.Background(), 6429840)
	wc, d := hs.WinCountReward, hs.Deltas
	if err != nil {
		t.Fatalf("旧口径可用时不应报错: %s", err)
	}
	if !wc.Equal(decimal.RequireFromString("20000000000000000000")) { // 100 FIL / 5
		t.Fatalf("应回退旧口径 20 FIL，得到 %s", wc)
	}
	if d.OK {
		t.Fatal("上游报错三流应 OK=false")
	}
	if agg.calls != 1 {
		t.Fatalf("回退路径也必须只调 1 次 reward_streams，实际 %d 次", agg.calls)
	}
	if !strings.Contains(logs.String(), "回退旧口径") {
		t.Fatalf("上游报错必须打 WARN，实际日志:\n%s", logs.String())
	}
}
