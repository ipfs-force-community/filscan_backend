package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

// fakeAgg 只实现本项目用到的 3 个方法，其余经 embedded interface 占位
// （测试不会调用它们；调用即 panic，属于测试本身写错）。
type fakeAgg struct {
	londobell.Agg

	minerBlockRewardCalls  int
	minersBlockRewardCalls int
	winCountCalls          int

	minerBlockRewardResult  []*londobell.MinerBlockReward
	minersBlockRewardResult []*londobell.MinersBlockReward
	winCountResult          []*londobell.MinerWinCount
	err                     error
}

func (f *fakeAgg) MinerBlockReward(_ context.Context, _ chain.SmartAddress, _ types.Filters) ([]*londobell.MinerBlockReward, error) {
	f.minerBlockRewardCalls++
	return f.minerBlockRewardResult, f.err
}

func (f *fakeAgg) MinersBlockReward(_ context.Context, _ chain.Epoch, _ chain.Epoch) ([]*londobell.MinersBlockReward, error) {
	f.minersBlockRewardCalls++
	return f.minersBlockRewardResult, f.err
}

func (f *fakeAgg) WinCount(_ context.Context, _ chain.Epoch, _ chain.Epoch) ([]*londobell.MinerWinCount, error) {
	f.winCountCalls++
	return f.winCountResult, f.err
}

type fakeReader struct {
	minerBlockRewardCalls  int
	minersBlockRewardCalls int
	winCountCalls          int

	rows  []*bo.MinerEpochReward
	wins  []*bo.AccWinCount
	err   error
	miner string
	start chain.Epoch
	end   chain.Epoch
}

func (f *fakeReader) MinerBlockRewardRange(_ context.Context, miner string, start, end chain.Epoch) ([]*bo.MinerEpochReward, error) {
	f.minerBlockRewardCalls++
	f.miner, f.start, f.end = miner, start, end
	return f.rows, f.err
}

func (f *fakeReader) MinersBlockRewardRange(_ context.Context, start, end chain.Epoch) ([]*bo.MinerEpochReward, error) {
	f.minersBlockRewardCalls++
	f.start, f.end = start, end
	return f.rows, f.err
}

func (f *fakeReader) MinerWinCountsRange(_ context.Context, start, end chain.Epoch) ([]*bo.AccWinCount, error) {
	f.winCountCalls++
	f.start, f.end = start, end
	return f.wins, f.err
}

func epochPtr(v chain.Epoch) *chain.Epoch { return &v }

// 开关全关时必须原样返回（同一个 agg 实例），且一次 SQL 都不发 —— 这是「零行为变化」的硬保证。
func TestPgRewardAggDisabledPassesThrough(t *testing.T) {
	agg := &fakeAgg{}
	got := NewPgRewardAggWithReader(agg, &fakeReader{err: errors.New("reader 不该被调用")}, PgRewardOptions{})
	if got != londobell.Agg(agg) {
		t.Fatalf("全关时应原样返回入参 agg，得到 %T", got)
	}

	// 配置入口（老配置文件：没有 [feature] 段）同样原样返回。
	fromConf := NewPgRewardAgg(agg, &gorm.DB{}, &config.Config{})
	if fromConf != londobell.Agg(agg) {
		t.Fatalf("配置无 [feature] 段时应原样返回入参 agg，得到 %T", fromConf)
	}

	start, end := chain.Epoch(100), chain.Epoch(101)
	if _, err := got.MinersBlockReward(context.Background(), start, end); err != nil {
		t.Fatalf("透传失败: %s", err)
	}
	if agg.minersBlockRewardCalls != 1 {
		t.Errorf("应调用聚合器 1 次，实际 %d", agg.minersBlockRewardCalls)
	}
}

// 单矿工出块奖励：字段映射 + 金额原样（attoFIL 大整数不许被换算/截断）+ 区间与地址形态。
func TestPgRewardMinerBlockRewardUsesPg(t *testing.T) {
	agg := &fakeAgg{err: errors.New("开关打开时不该调用聚合器")}
	reader := &fakeReader{rows: []*bo.MinerEpochReward{
		{Epoch: 100, Miner: "f01234", Reward: decimal.RequireFromString("4860000000000000000"), BlockCount: 2},
		{Epoch: 102, Miner: "f01234", Reward: decimal.RequireFromString("2430000000000000000"), BlockCount: 1},
	}}
	wrapped := NewPgRewardAggWithReader(agg, reader, PgRewardOptions{MinerBlockReward: true})

	rows, err := wrapped.MinerBlockReward(context.Background(),
		chain.SmartAddress("01234"), types.Filters{Start: epochPtr(100), End: epochPtr(103), Index: 0, Limit: 500})
	if err != nil {
		t.Fatalf("读 PG 失败: %s", err)
	}
	if agg.minerBlockRewardCalls != 0 {
		t.Errorf("开关打开时不应回落到聚合器，实际调用 %d 次", agg.minerBlockRewardCalls)
	}
	if reader.minerBlockRewardCalls != 1 {
		t.Errorf("应读 PG 1 次，实际 %d", reader.minerBlockRewardCalls)
	}
	if reader.miner != "f01234" {
		t.Errorf("PG miner 入参应为带前缀形态 f01234，得到 %q", reader.miner)
	}
	if reader.start != 100 || reader.end != 103 {
		t.Errorf("PG 区间应为 [100,103)，得到 [%d,%d)", reader.start, reader.end)
	}
	if len(rows) != 2 {
		t.Fatalf("应返回 2 行，得到 %d", len(rows))
	}
	if rows[0].Id != 100 || rows[1].Id != 102 {
		t.Errorf("Id 应为 epoch，得到 %d / %d", rows[0].Id, rows[1].Id)
	}
	if rows[0].TotalBlockReward.String() != "4860000000000000000" {
		t.Errorf("金额必须原样（attoFIL 大整数），得到 %s", rows[0].TotalBlockReward.String())
	}
	if rows[1].BlockCount != 1 {
		t.Errorf("BlockCount 映射错误：%d", rows[1].BlockCount)
	}
}

// 无区间（Start/End 为 nil）时不能拿 nil 去查 SQL，必须保持老路径。
func TestPgRewardMinerBlockRewardNilRangeFallsBack(t *testing.T) {
	agg := &fakeAgg{minerBlockRewardResult: []*londobell.MinerBlockReward{{Id: 1}}}
	reader := &fakeReader{err: errors.New("不该被调用")}
	wrapped := NewPgRewardAggWithReader(agg, reader, PgRewardOptions{MinerBlockReward: true})

	rows, err := wrapped.MinerBlockReward(context.Background(), chain.SmartAddress("f01234"), types.Filters{})
	if err != nil {
		t.Fatalf("回落失败: %s", err)
	}
	if reader.minerBlockRewardCalls != 0 {
		t.Errorf("nil 区间不应查 PG，实际调用 %d 次", reader.minerBlockRewardCalls)
	}
	if agg.minerBlockRewardCalls != 1 || len(rows) != 1 {
		t.Errorf("应回落聚合器，calls=%d rows=%d", agg.minerBlockRewardCalls, len(rows))
	}
}

// PG 出错必须回落聚合器（宁慢不空），并把两边都记录清楚。
func TestPgRewardFallsBackOnError(t *testing.T) {
	agg := &fakeAgg{
		minersBlockRewardResult: []*londobell.MinersBlockReward{{Id: londobell.EpochMiner{Epoch: 7, Miner: "01234"}}},
		winCountResult:          []*londobell.MinerWinCount{{Id: "01234", TotalWinCount: 3}},
	}
	reader := &fakeReader{err: errors.New("relation does not exist")}
	wrapped := NewPgRewardAggWithReader(agg, reader, PgRewardOptions{MinersBlockReward: true, MinerWinCount: true})

	if rows, err := wrapped.MinersBlockReward(context.Background(), 7, 8); err != nil || len(rows) != 1 {
		t.Fatalf("miners_blockreward 应回落聚合器: err=%v rows=%d", err, len(rows))
	}
	if rows, err := wrapped.WinCount(context.Background(), 7, 8); err != nil || len(rows) != 1 {
		t.Fatalf("wincount 应回落聚合器: err=%v rows=%d", err, len(rows))
	}
	if agg.minersBlockRewardCalls != 1 || agg.winCountCalls != 1 {
		t.Errorf("回落次数不对: miners=%d win=%d", agg.minersBlockRewardCalls, agg.winCountCalls)
	}
}

// PG 空结果 = nil（与聚合器空 data 数组反序列化后的形态一致），不能是「空但非 nil」。
func TestPgRewardEmptyResultIsNil(t *testing.T) {
	agg := &fakeAgg{}
	wrapped := NewPgRewardAggWithReader(agg, &fakeReader{}, PgRewardOptions{
		MinerBlockReward: true, MinersBlockReward: true, MinerWinCount: true,
	})

	if rows, err := wrapped.MinerBlockReward(context.Background(), chain.SmartAddress("f01234"),
		types.Filters{Start: epochPtr(1), End: epochPtr(2)}); err != nil || rows != nil {
		t.Errorf("空结果应为 nil slice: err=%v rows=%v", err, rows)
	}
	if rows, err := wrapped.MinersBlockReward(context.Background(), 1, 2); err != nil || rows != nil {
		t.Errorf("空结果应为 nil slice: err=%v rows=%v", err, rows)
	}
	if rows, err := wrapped.WinCount(context.Background(), 1, 2); err != nil || rows != nil {
		t.Errorf("空结果应为 nil slice: err=%v rows=%v", err, rows)
	}
	if agg.minerBlockRewardCalls+agg.minersBlockRewardCalls+agg.winCountCalls != 0 {
		t.Errorf("空结果不应预判为缺口去回落聚合器")
	}
}

// wincount：Id 给不带前缀形态（与聚合器 _id 一致）、TotalGasReward 恒 0（PG 无该列）。
func TestPgRewardWinCountMapping(t *testing.T) {
	agg := &fakeAgg{}
	reader := &fakeReader{wins: []*bo.AccWinCount{{Miner: "f01234", WinCount: 12}}}
	wrapped := NewPgRewardAggWithReader(agg, reader, PgRewardOptions{MinerWinCount: true})

	rows, err := wrapped.WinCount(context.Background(), 100, 200)
	if err != nil || len(rows) != 1 {
		t.Fatalf("读 PG 失败: err=%v rows=%d", err, len(rows))
	}
	if rows[0].Id != "01234" {
		t.Errorf("Id 应为不带前缀形态 01234，得到 %q", rows[0].Id)
	}
	if rows[0].TotalWinCount != 12 {
		t.Errorf("TotalWinCount 映射错误: %d", rows[0].TotalWinCount)
	}
	if !rows[0].TotalGasReward.IsZero() {
		t.Errorf("TotalGasReward 无 PG 来源，应恒 0，得到 %s", rows[0].TotalGasReward.String())
	}
	if rows[0].TotalGasReward.String() != "0" {
		t.Errorf("TotalGasReward 文本形态应为 0，得到 %q", rows[0].TotalGasReward.String())
	}
}

// 三个开关互相独立：只开一个不能影响另外两个。
func TestPgRewardSwitchesAreIndependent(t *testing.T) {
	agg := &fakeAgg{}
	reader := &fakeReader{}
	wrapped := NewPgRewardAggWithReader(agg, reader, PgRewardOptions{MinersBlockReward: true})

	if _, err := wrapped.MinersBlockReward(context.Background(), 1, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.MinerBlockReward(context.Background(), chain.SmartAddress("f01234"),
		types.Filters{Start: epochPtr(1), End: epochPtr(2)}); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.WinCount(context.Background(), 1, 2); err != nil {
		t.Fatal(err)
	}
	if reader.minersBlockRewardCalls != 1 {
		t.Errorf("开的那个应走 PG，实际 %d", reader.minersBlockRewardCalls)
	}
	if reader.minerBlockRewardCalls != 0 || reader.winCountCalls != 0 {
		t.Errorf("没开的两个不应走 PG: miner=%d win=%d", reader.minerBlockRewardCalls, reader.winCountCalls)
	}
	if agg.minerBlockRewardCalls != 1 || agg.winCountCalls != 1 {
		t.Errorf("没开的两个应走聚合器: miner=%d win=%d", agg.minerBlockRewardCalls, agg.winCountCalls)
	}
}

// db 为空（例如单测/未注入）时不得装饰：宁可走聚合器。
func TestNewPgRewardAggWithoutDB(t *testing.T) {
	agg := &fakeAgg{}
	if got := NewPgRewardAgg(agg, nil, nil); got != londobell.Agg(agg) {
		t.Errorf("db=nil 时应原样返回入参 agg")
	}
}
