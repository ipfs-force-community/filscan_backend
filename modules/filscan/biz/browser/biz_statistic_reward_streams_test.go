package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 真实抓取的 calibnet 相邻两行（升级高度 4109133 前后；见 pkg/londobell/nv29_reward_test.go）。
// prev 是 v18（拆分前毛值），cur 是 v19（按权重拆分后）。
const (
	realV18Row = `{"Epoch":4107459,"TotalStoragePowerReward":"110469517489432681170003963","TotalMintedReward":"0","TotalBurnMinted":"0","TotalExplicitMinted":"0"}`
	realV19Row = `{"Epoch":4110339,"TotalStoragePowerReward":"0","TotalMintedReward":"110534789174400147324658484","TotalBurnMinted":"3338","TotalExplicitMinted":"1664003218449871124998"}`
)

func mustStream(t *testing.T, raw string) *londobell.RewardStream {
	t.Helper()
	var s londobell.RewardStream
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("构造样本失败: %s", err)
	}
	return &s
}

type fakeRewardStreamsAgg struct {
	streams []*londobell.RewardStream
	err     error

	calls int
	start chain.Epoch
	end   chain.Epoch
}

func (f *fakeRewardStreamsAgg) RewardStreams(_ context.Context, start, end chain.Epoch) ([]*londobell.RewardStream, error) {
	f.calls++
	f.start, f.end = start, end
	return f.streams, f.err
}

type fakeRewardStreamsSyncer struct {
	repository.SyncerGetter
	epoch int64
	err   error
}

func (f *fakeRewardStreamsSyncer) GetSyncer(_ context.Context, _ string) (*po.SyncSyncer, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &po.SyncSyncer{Name: "chain", Epoch: f.epoch}, nil
}

// v18→v19 真实相邻两行的差分：miner=ΔMinerMinted、service=ΔExplicit、burn=ΔBurn，单位 attoFIL。
func TestBuildRewardStreamItemsRealNV29Pair(t *testing.T) {
	items := buildRewardStreamItems([]*londobell.RewardStream{
		mustStream(t, realV18Row),
		mustStream(t, realV19Row),
	})
	if len(items) != 1 {
		t.Fatalf("相邻两行应得到 1 个数据点，得到 %d", len(items))
	}
	it := items[0]
	if it.Epoch != 4110339 {
		t.Fatalf("数据点的 epoch 应取区间末端 4110339，得到 %d", it.Epoch)
	}
	if it.BlockTime != chain.Epoch(4110339).Time().Unix() {
		t.Fatalf("block_time 错: got %d", it.BlockTime)
	}

	// 期望值＝真实计数器差分的原始值（单位 attoFIL，与统计页其它曲线一致）。
	wantMiner := decimal.RequireFromString("63607681749016283526185")
	wantService := decimal.RequireFromString("1664003218449871124998")
	wantBurn := decimal.RequireFromString("3338")
	wantTotal := decimal.RequireFromString("65271684967466154654521")

	if !it.Miner.Equal(wantMiner) {
		t.Fatalf("miner 差分错: got %s want %s", it.Miner, wantMiner)
	}
	if !it.Service.Equal(wantService) {
		t.Fatalf("service 差分错: got %s want %s", it.Service, wantService)
	}
	if !it.Burn.Equal(wantBurn) {
		t.Fatalf("burn 差分错: got %s want %s", it.Burn, wantBurn)
	}
	if !it.Total.Equal(wantTotal) {
		t.Fatalf("total 错: got %s want %s", it.Total, wantTotal)
	}
	assertRewardStreamInvariant(t, it)
}

// v18 分支：只有矿工实收一条线，service/burn 必须恒 0。
func TestBuildRewardStreamItemsV18Only(t *testing.T) {
	prev := londobell.RewardStream{Epoch: 1000, TotalStoragePowerReward: decimal.RequireFromString("1000000000000000000000")}
	cur := londobell.RewardStream{Epoch: 1120, TotalStoragePowerReward: decimal.RequireFromString("1001000000000000000000")}
	items := buildRewardStreamItems([]*londobell.RewardStream{&prev, &cur})
	if len(items) != 1 {
		t.Fatalf("应得到 1 个数据点，得到 %d", len(items))
	}
	it := items[0]
	if !it.Miner.Equal(decimal.RequireFromString("1000000000000000000")) { // 1e18 attoFIL = 1 FIL
		t.Fatalf("v18 miner 应为 1e18 attoFIL，得到 %s", it.Miner)
	}
	if !it.Service.IsZero() || !it.Burn.IsZero() {
		t.Fatalf("v18 service/burn 必须为 0，得到 service=%s burn=%s", it.Service, it.Burn)
	}
	if !it.Total.Equal(decimal.RequireFromString("1000000000000000000")) {
		t.Fatalf("v18 total 应等于 miner，得到 %s", it.Total)
	}
	assertRewardStreamInvariant(t, it)
}

// 空序列 / 单行：返回空数组（不是 nil，也不是错误）。
func TestBuildRewardStreamItemsEmpty(t *testing.T) {
	for name, in := range map[string][]*londobell.RewardStream{
		"nil":      nil,
		"empty":    {},
		"single":   {mustStream(t, realV19Row)},
		"nilElems": {nil, nil},
	} {
		items := buildRewardStreamItems(in)
		if items == nil {
			t.Fatalf("%s: 应返回空数组而不是 nil", name)
		}
		if len(items) != 0 {
			t.Fatalf("%s: 应无数据点，得到 %d", name, len(items))
		}
	}
}

// 乱序输入也要按 Epoch 升序求差（契约 A 保证升序，这里兜住上游变化）。
func TestBuildRewardStreamItemsSortsInput(t *testing.T) {
	prev := londobell.RewardStream{Epoch: 1120, TotalStoragePowerReward: decimal.RequireFromString("1001000000000000000000")}
	cur := londobell.RewardStream{Epoch: 1000, TotalStoragePowerReward: decimal.RequireFromString("1000000000000000000000")}
	items := buildRewardStreamItems([]*londobell.RewardStream{&prev, &cur}) // 故意倒序
	if len(items) != 1 {
		t.Fatalf("应得到 1 个数据点，得到 %d", len(items))
	}
	if items[0].Epoch != 1120 {
		t.Fatalf("数据点应落在较大 epoch=1120，得到 %d", items[0].Epoch)
	}
}

// 负增量（快照回滚/数据异常）：按 0 计入并打 WARN，保证各分量非负。
func TestBuildRewardStreamItemsClampsNegative(t *testing.T) {
	logs := captureBizLogs(t)
	prev := londobell.RewardStream{Epoch: 1000, TotalStoragePowerReward: decimal.RequireFromString("2000000000000000000000")}
	cur := londobell.RewardStream{Epoch: 1120, TotalStoragePowerReward: decimal.RequireFromString("1900000000000000000000")}
	items := buildRewardStreamItems([]*londobell.RewardStream{&prev, &cur})
	if len(items) != 1 {
		t.Fatalf("应得到 1 个数据点，得到 %d", len(items))
	}
	it := items[0]
	if !it.Miner.IsZero() || !it.Total.IsZero() {
		t.Fatalf("负增量应被归零，得到 miner=%s total=%s", it.Miner, it.Total)
	}
	assertRewardStreamInvariant(t, it)
	if !strings.Contains(logs.String(), "出现负增量") {
		t.Fatalf("负增量必须打 WARN，实际日志:\n%s", logs.String())
	}
}

// nv29_epoch 映射：未排期（UpgradeHeightUnscheduled）/非正 ⇒ 0；已排期 ⇒ 原值。
func TestNv29EpochOrZero(t *testing.T) {
	if got := nv29EpochOrZero(999999999999999); got != 0 {
		t.Fatalf("未排期应输出 0，得到 %d", got)
	}
	if got := nv29EpochOrZero(0); got != 0 {
		t.Fatalf("0 应输出 0，得到 %d", got)
	}
	if got := nv29EpochOrZero(-5); got != 0 {
		t.Fatalf("负值应输出 0，得到 %d", got)
	}
	if got := nv29EpochOrZero(4109133); got != 4109133 {
		t.Fatalf("已排期应原样输出，得到 %d", got)
	}
}

// 24h 档：窗口应解析为 [head-2880, head+1)，且 nv29_epoch 取自本仓构建常量。
func TestRewardStreamsResolvesWindow24h(t *testing.T) {
	const head int64 = 6429840 // 120 的整数倍
	agg := &fakeRewardStreamsAgg{streams: []*londobell.RewardStream{
		mustStream(t, realV18Row), mustStream(t, realV19Row),
	}}
	biz := NewStatisticRewardStreamsBiz(&fakeRewardStreamsSyncer{epoch: head}, agg)

	resp, err := biz.RewardStreams(context.Background(), filscan.RewardStreamsRequest{Interval: "24h"})
	if err != nil {
		t.Fatalf("调用失败: %s", err)
	}
	if agg.start != chain.Epoch(head-2880) || agg.end != chain.Epoch(head+1) {
		t.Fatalf("24h 窗口错: got [%d,%d) want [%d,%d)", agg.start, agg.end, head-2880, head+1)
	}
	if resp.Nv29Epoch != nv29EpochOrZero(message_detail.UpgradeSolsticeHeight.Int64()) {
		t.Fatalf("nv29_epoch 未取本仓构建常量: got %d", resp.Nv29Epoch)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("应返回 1 个数据点，得到 %d", len(resp.Items))
	}
}

// 7d 档窗口 = head - 7*2880。
func TestRewardStreamsResolvesWindow7d(t *testing.T) {
	const head int64 = 6429840
	agg := &fakeRewardStreamsAgg{}
	biz := NewStatisticRewardStreamsBiz(&fakeRewardStreamsSyncer{epoch: head}, agg)

	if _, err := biz.RewardStreams(context.Background(), filscan.RewardStreamsRequest{Interval: "7d"}); err != nil {
		t.Fatalf("调用失败: %s", err)
	}
	if agg.start != chain.Epoch(head-7*2880) || agg.end != chain.Epoch(head+1) {
		t.Fatalf("7d 窗口错: got [%d,%d)", agg.start, agg.end)
	}
}

func TestRewardStreamsInvalidInterval(t *testing.T) {
	biz := NewStatisticRewardStreamsBiz(&fakeRewardStreamsSyncer{epoch: 100}, &fakeRewardStreamsAgg{})
	if _, err := biz.RewardStreams(context.Background(), filscan.RewardStreamsRequest{Interval: "nope"}); err == nil {
		t.Fatal("非法 interval 应报错")
	}
}

func TestRewardStreamsAggError(t *testing.T) {
	agg := &fakeRewardStreamsAgg{err: errors.New("boom")}
	biz := NewStatisticRewardStreamsBiz(&fakeRewardStreamsSyncer{epoch: 100}, agg)
	if _, err := biz.RewardStreams(context.Background(), filscan.RewardStreamsRequest{Interval: "24h"}); err == nil {
		t.Fatal("聚合器报错应向上返回")
	}
}

func TestRewardStreamsSyncerError(t *testing.T) {
	biz := NewStatisticRewardStreamsBiz(&fakeRewardStreamsSyncer{err: errors.New("db down")}, &fakeRewardStreamsAgg{})
	if _, err := biz.RewardStreams(context.Background(), filscan.RewardStreamsRequest{Interval: "24h"}); err == nil {
		t.Fatal("同步器高度取不到应报错")
	}
}

// assertRewardStreamInvariant total = miner + service + burn 且各分量非负。
func assertRewardStreamInvariant(t *testing.T, it *filscan.RewardStreamItem) {
	t.Helper()
	if it.Miner.IsNegative() || it.Service.IsNegative() || it.Burn.IsNegative() || it.Total.IsNegative() {
		t.Fatalf("分量/总量出现负值: %+v", it)
	}
	sum := it.Miner.Add(it.Service).Add(it.Burn)
	if !sum.Equal(it.Total) {
		t.Fatalf("不变式被破坏: total=%s != miner+service+burn=%s", it.Total, sum)
	}
}
