package minergasestimatecmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

// 本文件用**真实的 CalEstimateMinerGas** 跑完整条 Dry 清单管线，是
// 「miner-gas-estimate 子命令真能补 chain.base_gas_costs.sector_fee32/64、只改派生表、
// --no-write 下零写入、输入（最近 960 高度的 chain.base_gas_costs）为空时零写入」的主证据。

const (
	estHeadEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	estReplayA   = 6357122
	estReplayB   = 6357123
)

// estRows 造「最近 960 高度窗口」里的 base_gas_costs 行（items[0].BaseGas 会被当作 baseFee）。
// 期望值：limit32 = 100+100, count32 = 2 ⇒ 均值 100 ⇒ sector_fee32 = 1 × 100 × (1024/32) = 3200；
//
//	limit64 = 50+50,  count64 = 2 ⇒ 均值 50  ⇒ sector_fee64 = 1 × 50  × (1024/64) = 800。
func estRows() []*po.BaseGasCostPo {
	return []*po.BaseGasCostPo{
		{Epoch: estReplayA, BaseGas: decimal.NewFromInt(1), AvgGasLimit32: decimal.NewFromInt(100), AvgGasLimit64: decimal.NewFromInt(50)},
		{Epoch: estReplayA - 120, BaseGas: decimal.NewFromInt(1), AvgGasLimit32: decimal.NewFromInt(100), AvgGasLimit64: decimal.NewFromInt(50)},
	}
}

// estReplay 一次回放的装配结果
type estReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *offlinereplay.FakeAgg
	adapter *offlinereplay.FakeAdapter
	inner   *fakeTraceRepo
}

// buildEstReplay 用真实计算器 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配清单回放。
func buildEstReplay(t *testing.T, noWrite bool, heights ...int64) *estReplay {
	t.Helper()

	inner := &fakeTraceRepo{rows: estRows()}
	restore := swapTraceRepo(inner)
	t.Cleanup(restore)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)

	target, writes := buildTarget(db, noWrite)

	text := ""
	for _, h := range heights {
		text += fmt.Sprintf("%d\n", h)
	}
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)

	return &estReplay{
		rec: rec, db: db, plan: plan, target: target, writes: writes,
		agg: offlinereplay.NewFakeAgg(estHeadEpoch, nil), adapter: &offlinereplay.FakeAdapter{}, inner: inner,
	}
}

// execute 跑一次回放并返回打印出来的报告
func (r *estReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureEstStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

func captureEstStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	rd, wr, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wr
	defer func() { os.Stdout = old }()

	fn()
	require.NoError(t, wr.Close())
	out, err := io.ReadAll(rd)
	require.NoError(t, err)
	return string(out)
}

// --no-write：真实计算器跑满清单，chain.base_gas_costs 的两列更新只被计数、一次都没转发到真仓储，
// 且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）；也不该有任何 Traces 调用。
func TestMinerGasEstimateNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildEstReplay(t, true, estReplayA, estReplayB)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Empty(t, rp.agg.EpochsRequested(), "本计算器不读 traces ⇒ 不该有 Traces 调用")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 每高度一次「改 1 行」的 UPDATE
	require.Equal(t, offlinereplay.WriteStat{Table: TableBaseGasCosts, Calls: 2, Rows: 2}, stats[TableBaseGasCosts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerGasFees}, stats[TableMinerGasFees],
		"本命令不跑 trace-task ⇒ 不碰 chain.miner_gas_fees")
	require.Equal(t, offlinereplay.WriteStat{Table: TableMethodGasFees}, stats[TableMethodGasFees])

	require.Contains(t, report, "同步器/任务     : chain / calc-estimate-miner-gas")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "chain.base_gas_costs 写 2 次/2 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 2 行")
}

// 真写：每高度一次扇区费更新（epoch 就是清单高度），算出来的两个值符合公式；
// 读窗口是「最近 960 个高度」（gtStart = 高度 - 120×8）。
func TestMinerGasEstimateDryModeUpdatesSectorFeeColumns(t *testing.T) {
	rp := buildEstReplay(t, false, estReplayA, estReplayB)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.updates, 2, "每个高度一次 UpdateBaseGasCostSectorGas")
	for i, up := range rp.inner.updates {
		wantEpoch := []int64{estReplayA, estReplayB}[i]
		require.Equal(t, wantEpoch, up.epoch.Int64(), "更新的高度必须就是清单里的高度")
		require.Equal(t, 0, up.fee32.Cmp(decimal.NewFromInt(3200)), "sector_fee32 = baseFee × 均值 × 1024/32")
		require.Equal(t, 0, up.fee64.Cmp(decimal.NewFromInt(800)), "sector_fee64 = baseFee × 均值 × 1024/64")
	}

	require.Equal(t, []int64{estReplayA - 960, estReplayB - 960},
		[]int64{rp.inner.window[0].Int64(), rp.inner.window[1].Int64()},
		"读窗口起点 = 当前高度 - 120×8（最近 960 个高度）")

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// 输入为空的样子（这正是必须先跑 miner-gas-fees 的原因）：最近 960 高度窗口里一行都没有时，
// 计算器直接返回、什么都不写（不是写 0，而是**不写**）。
func TestMinerGasEstimateEmptyWindowWritesNothing(t *testing.T) {
	rp := buildEstReplay(t, false, estReplayA)
	rp.inner.rows = nil

	report := rp.execute(t, offlinereplay.RunOptions{})

	require.Empty(t, rp.inner.updates, "窗口为空 ⇒ 计算器提前返回，一次 UPDATE 都不该有")
	require.Equal(t, 1, len(rp.inner.window), "仍然读了窗口（才发现是空的）")
	require.Contains(t, report, "逐高度结果     : 已跑 1/1 个高度；放弃 0 个；未跑 0 个",
		"报告显示「跑完了」，但一行都没写 —— 这就是必须先跑 miner-gas-fees 的原因")
	require.Empty(t, rp.rec.Statements())
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestMinerGasEstimateRejectsHeightsAboveHead(t *testing.T) {
	rp := buildEstReplay(t, false, estReplayA, estHeadEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", estHeadEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}
