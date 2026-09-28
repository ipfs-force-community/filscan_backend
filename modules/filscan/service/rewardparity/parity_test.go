package rewardparity

import (
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

var testParams = Params{Miner: chain.SmartAddress("f01234"), Start: 100, End: 200}

// 两条路径逐字段一致 → PASS。
func TestCompareMinerBlockRewardParity(t *testing.T) {
	agg := []*londobell.MinerBlockReward{
		{Id: 100, TotalBlockReward: decimal.RequireFromString("4860000000000000000"), BlockCount: 2},
		{Id: 102, TotalBlockReward: decimal.RequireFromString("2430000000000000000"), BlockCount: 1},
	}
	pg := []*bo.MinerEpochReward{
		{Epoch: 100, Miner: "f01234", Reward: decimal.RequireFromString("4860000000000000000"), BlockCount: 2},
		{Epoch: 102, Miner: "f01234", Reward: decimal.RequireFromString("2430000000000000000"), BlockCount: 1},
	}
	r := CompareMinerBlockReward(testParams, agg, pg, 10)
	if !r.Pass(true) {
		t.Fatalf("应 PASS，实际: diff=%d aggOnly=%d pgOnly=%d", r.DiffCount, r.AggOnlyCount(), r.PgOnlyCount())
	}
	if r.AggSumReward.String() != "7290000000000000000" || r.PgSumReward.String() != "7290000000000000000" {
		t.Errorf("总量应一致，得到 agg=%s pg=%s", r.AggSumReward.String(), r.PgSumReward.String())
	}
}

// 金额不等 → FAIL（ValueDiffCount 命中），并且必须打印出两边的原始文本。
func TestCompareMinerBlockRewardValueMismatch(t *testing.T) {
	agg := []*londobell.MinerBlockReward{{Id: 100, TotalBlockReward: decimal.RequireFromString("1"), BlockCount: 1}}
	pg := []*bo.MinerEpochReward{{Epoch: 100, Miner: "f01234", Reward: decimal.RequireFromString("2"), BlockCount: 1}}
	r := CompareMinerBlockReward(testParams, agg, pg, 10)
	if r.Pass(true) {
		t.Fatal("金额不等必须 FAIL")
	}
	if r.ValueDiffCount != 1 {
		t.Errorf("valueDiff 应为 1，得到 %d", r.ValueDiffCount)
	}
	if len(r.Diffs) != 1 || r.Diffs[0].Agg != "1" || r.Diffs[0].Pg != "2" || r.Diffs[0].Field != "TotalBlockReward" {
		t.Errorf("差异样例不对: %+v", r.Diffs)
	}
}

// 覆盖缺口（聚合器有、PG 没有）必须 FAIL 并给出点位。
func TestCompareCoverageGap(t *testing.T) {
	agg := []*londobell.MinerBlockReward{
		{Id: 100, TotalBlockReward: decimal.NewFromInt(1), BlockCount: 1},
		{Id: 101, TotalBlockReward: decimal.NewFromInt(1), BlockCount: 1},
	}
	pg := []*bo.MinerEpochReward{{Epoch: 100, Miner: "f01234", Reward: decimal.NewFromInt(1), BlockCount: 1}}
	r := CompareMinerBlockReward(testParams, agg, pg, 10)
	if r.Pass(true) {
		t.Fatal("缺点位必须 FAIL")
	}
	if r.AggOnlyCount() != 1 || r.AggOnly()[0] != "101" {
		t.Errorf("aggOnly 应为 [101]，得到 %v", r.AggOnly())
	}
}

// PG 多出点位（重复行没被去干净 / 口径不同）同样 FAIL。
func TestComparePgOnly(t *testing.T) {
	agg := []*londobell.MinerBlockReward{{Id: 100, TotalBlockReward: decimal.NewFromInt(1), BlockCount: 1}}
	pg := []*bo.MinerEpochReward{
		{Epoch: 100, Miner: "f01234", Reward: decimal.NewFromInt(1), BlockCount: 1},
		{Epoch: 101, Miner: "f01234", Reward: decimal.NewFromInt(1), BlockCount: 1},
	}
	r := CompareMinerBlockReward(testParams, agg, pg, 10)
	if r.PgOnlyCount() != 1 || r.PgOnly()[0] != "101" {
		t.Errorf("pgOnly 应为 [101]，得到 %v", r.PgOnly())
	}
}

// 点位 key 的地址形态必须归一：聚合器给 0…（不带前缀），PG 存 f0…（带前缀），不能因此误报缺口。
func TestCompareMinersBlockRewardNormalizesAddressForm(t *testing.T) {
	agg := []*londobell.MinersBlockReward{
		{Id: londobell.EpochMiner{Epoch: 100, Miner: "01234"}, TotalBlockReward: decimal.NewFromInt(7), BlockCount: 3},
	}
	pg := []*bo.MinerEpochReward{{Epoch: 100, Miner: "f01234", Reward: decimal.NewFromInt(7), BlockCount: 3}}
	r := CompareMinersBlockReward(testParams, agg, pg, 10)
	if !r.Pass(true) {
		t.Fatalf("地址形态归一后应 PASS: aggOnly=%v pgOnly=%v diffs=%+v", r.AggOnly(), r.PgOnly(), r.Diffs)
	}
}

// wincount：WinCount 必须逐值相等；gasReward 非 0 时显式记为差异（PG 无来源）。
func TestCompareWinCountGasRewardGap(t *testing.T) {
	agg := []*londobell.MinerWinCount{
		{Id: "01234", TotalWinCount: 5, TotalGasReward: decimal.NewFromInt(100)},
	}
	pg := []*bo.AccWinCount{{Miner: "f01234", WinCount: 5}}
	r := CompareWinCount(testParams, agg, pg, 10)
	if r.Pass(false) {
		t.Fatal("gasReward 有值而 PG 无来源时必须 FAIL")
	}
	if len(r.Diffs) != 1 || r.Diffs[0].Field != "TotalGasReward" {
		t.Errorf("应有一条 TotalGasReward 差异，得到 %+v", r.Diffs)
	}
	if len(r.Unresolved) == 0 {
		t.Error("必须声明 TotalGasReward 无 PG 来源")
	}
}

// gasReward 本来就是 0（老管线不产出该字段）时不应产生差异。
func TestCompareWinCountZeroGasRewardIsClean(t *testing.T) {
	agg := []*londobell.MinerWinCount{
		{Id: "01234", TotalWinCount: 5, TotalGasReward: decimal.Zero},
	}
	pg := []*bo.AccWinCount{{Miner: "f01234", WinCount: 5}}
	r := CompareWinCount(testParams, agg, pg, 10)
	if !r.Pass(true) {
		t.Fatalf("应 PASS: diffs=%+v", r.Diffs)
	}
}

// WinCount 不等必须 FAIL，且 TotalGasReward 不影响判定。
func TestCompareWinCountMismatch(t *testing.T) {
	agg := []*londobell.MinerWinCount{{Id: "01234", TotalWinCount: 5, TotalGasReward: decimal.Zero}}
	pg := []*bo.AccWinCount{{Miner: "f01234", WinCount: 4}}
	r := CompareWinCount(testParams, agg, pg, 10)
	if r.ValueDiffCount != 1 || r.Pass(true) {
		t.Errorf("WinCount 不等应 FAIL 并计 1 条数值差异，得到 %d / pass=%v", r.ValueDiffCount, r.Pass(true))
	}
}

// 样例截断：计数是全量的，样例最多 maxExamples 条。
func TestExamplesAreCappedButCountsAreFull(t *testing.T) {
	var agg []*londobell.MinerBlockReward
	var pg []*bo.MinerEpochReward
	for i := int64(0); i < 50; i++ {
		agg = append(agg, &londobell.MinerBlockReward{Id: 100 + i, TotalBlockReward: decimal.NewFromInt(1), BlockCount: 1})
		pg = append(pg, &bo.MinerEpochReward{Epoch: 100 + i, Miner: "f01234", Reward: decimal.NewFromInt(2), BlockCount: 1})
	}
	r := CompareMinerBlockReward(testParams, agg, pg, 5)
	if r.ValueDiffCount != 50 {
		t.Errorf("数值差异计数应为全量 50，得到 %d", r.ValueDiffCount)
	}
	if len(r.Diffs) != 5 {
		t.Errorf("样例应被截断到 5，得到 %d", len(r.Diffs))
	}
}

// strictFormat 开关语义：仅文本不同不算 fatal，但 strict 模式下判 FAIL。
func TestStrictFormatSemantics(t *testing.T) {
	r := Result{Endpoint: EndpointWinCount, FormatOnlyDiff: 1}
	if r.Fatal() != 0 {
		t.Error("仅文本不同不应算 fatal")
	}
	if r.Pass(true) {
		t.Error("strict 模式下仅文本不同应 FAIL")
	}
	if !r.Pass(false) {
		t.Error("非 strict 模式下仅文本不同应 PASS")
	}
}

// 取数出错一律 FAIL（不允许「聚合器炸了所以跳过」被当成通过）。
func TestErrorsFail(t *testing.T) {
	r := Result{Endpoint: EndpointMinerBlockReward}
	r.AggErr = errFake{}
	if r.Pass(true) {
		t.Error("聚合器侧出错必须 FAIL")
	}
	r = Result{Endpoint: EndpointMinerBlockReward}
	r.PgErr = errFake{}
	if r.Pass(true) {
		t.Error("PG 侧出错必须 FAIL")
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake error" }
