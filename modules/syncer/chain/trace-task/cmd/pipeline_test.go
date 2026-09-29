package minergasfeecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/typer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/trace-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

// 本文件用**真实的 trace_task.Trace** 跑完整条 Dry 管线（含真实 SetTracesBuilder 注入 traces），
// 是「miner-gas-fees 子命令真能补 chain.miner_gas_fees / method_gas_fees / base_gas_costs、
// 只写派生表、--no-write 下零写入、必须注入 traces」的主证据。

const (
	segHeadEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	segReplayA   = 6357122
	segReplayB   = 6357123
)

// fakeTraceAgg 在框架假聚合器上加三个本链路要用的方法（Pre/Pro 聚合费、矿工扇区 Gas）。
// traces 由框架的 TracesFn 提供（SetTracesBuilder 会调它）。
type fakeTraceAgg struct {
	*offlinereplay.FakeAgg

	preEmpty bool // 造「聚合器取不到该高度」的场景（pre/pro/sector 三项全空 ⇒ 相关列偏低）
}

func (f *fakeTraceAgg) AggPreNetFee(_ context.Context, _, _ chain.Epoch) ([]*londobell.AggPreNetFee, error) {
	if f.preEmpty {
		return nil, nil
	}
	return []*londobell.AggPreNetFee{{Miner: "t0100", AggFee: decimal.NewFromInt(11)}}, nil
}

func (f *fakeTraceAgg) AggProNetFee(_ context.Context, _, _ chain.Epoch) ([]*londobell.AggProNetFee, error) {
	if f.preEmpty {
		return nil, nil
	}
	return []*londobell.AggProNetFee{{Miner: "t0100", AggFee: decimal.NewFromInt(22)}}, nil
}

func (f *fakeTraceAgg) MinerGasCost(_ context.Context, _, _ chain.Epoch) ([]*londobell.MinerGasCost, error) {
	if f.preEmpty {
		return nil, nil
	}
	return []*londobell.MinerGasCost{{ID: "t0100", GasCost: decimal.NewFromInt(33)}}, nil
}

// traces 造某高度的 traces：一条 PreCommitSector（带 GasCost，同时进 method_gas_fees 与
// MinerGasCostCalculator 的 32G 统计）、一条 SubmitWindowedPoSt（带 GasCost ⇒ 计入 wd_post_gas）。
func traces(epoch chain.Epoch) []*londobell.TraceMessage {
	return []*londobell.TraceMessage{
		{
			ID: "cid-1", Epoch: epoch.Int64(), To: "t0100", GasLimit: 1000, GasPremium: "1000000000",
			Detail:  &londobell.MessageDetail{Method: "PreCommitSector"},
			GasCost: &londobell.GasCost{TotalCost: decimal.NewFromInt(10), GasUsed: decimal.NewFromInt(5)},
		},
		{
			ID: "cid-2", Epoch: epoch.Int64(), To: "t0100", GasLimit: 2000,
			Detail:  &londobell.MessageDetail{Method: "SubmitWindowedPoSt"},
			GasCost: &londobell.GasCost{TotalCost: decimal.NewFromInt(20), GasUsed: decimal.NewFromInt(8)},
		},
	}
}

// segReplay 一次回放的装配结果
type segReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *fakeTraceAgg
	adapter *offlinereplay.FakeAdapter
	inner   *fakeTraceRepo
}

// buildSegReplay 用真实任务 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配清单回放。
// 扇区大小走假的：生产上它来自 chain.sync_miner_epochs + chain.miner_infos（回放时通常已是链头那行）。
func buildSegReplay(t *testing.T, noWrite bool, agg *fakeTraceAgg, heights ...int64) *segReplay {
	t.Helper()

	inner := &fakeTraceRepo{}
	restore := restoreTraceRepo(inner)
	t.Cleanup(restore)

	adapter := &offlinereplay.FakeAdapter{}

	restoreCalc := swapMinerGasCalc(adapter)
	t.Cleanup(restoreCalc)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)

	target, writes := buildTarget(db, adapter, noWrite)

	text := ""
	for _, h := range heights {
		text += fmt.Sprintf("%d\n", h)
	}
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)

	return &segReplay{rec: rec, db: db, plan: plan, target: target, writes: writes, agg: agg, adapter: adapter, inner: inner}
}

// swapMinerGasCalc 把 MinerGas 计算器换成「扇区大小来自假仓储」的版本
// （生产路径仍是 dal.NewChangeActorTaskDal + adapter 回退）
func swapMinerGasCalc(adapter londobell.Adapter) func() {
	old := newMinerGasCalc
	newMinerGasCalc = func(*gorm.DB, londobell.Adapter) *trace_task.MinerGasCostCalculator {
		return trace_task.NewMinerGasCostCalculator(typer.NewTyper(&fakeChangeActorRepo{size: 34359738368}, adapter))
	}
	return func() { newMinerGasCalc = old }
}

// execute 跑一次回放并返回打印出来的报告
func (r *segReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureSegStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

func captureSegStdout(t *testing.T, fn func()) string {
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

func newTraceAgg(head int64) *fakeTraceAgg {
	return &fakeTraceAgg{FakeAgg: offlinereplay.NewFakeAgg(head, traces)}
}

// --no-write：真实任务跑满清单，三张派生表的写入只被计数、一次都没转发到真仓储，
// 且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）。
// 本任务读 traces ⇒ 与 actor 链路相反，这里**必须**看到 Traces 调用。
func TestMinerGasFeesNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildSegReplay(t, true, newTraceAgg(segHeadEpoch), segReplayA, segReplayB)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Equal(t, []int64{segReplayA, segReplayB}, rp.agg.SortedEpochsRequested(),
		"trace-task 读 traces ⇒ 每个高度必须真的取一次 traces")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 每高度：base_gas_costs 1 行、method_gas_fees 2 行（两个方法）、miner_gas_fees 1 行（一个矿工）
	require.Equal(t, offlinereplay.WriteStat{Table: TableBaseGasCosts, Calls: 2, Rows: 2}, stats[TableBaseGasCosts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMethodGasFees, Calls: 2, Rows: 4}, stats[TableMethodGasFees])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerGasFees, Calls: 2, Rows: 2}, stats[TableMinerGasFees])

	require.Contains(t, report, "同步器/任务     : chain / trace-task")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "chain.miner_gas_fees 写 2 次/2 行；删除 0 次")
	require.Contains(t, report, "chain.method_gas_fees 写 2 次/4 行；删除 0 次")
	require.Contains(t, report, "chain.base_gas_costs 写 2 次/2 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 8 行")
	// traces 每高度 1 次（SetTracesBuilder），另有 tipset/parentTipset 各 1 次
	require.Contains(t, report, "Traces=")
}

// 不加 --no-write（真写通道 = 假仓储）时：三张派生表真写入，且写入的高度就是清单里的高度；
// Dry 依然保证不写同步指针 / 台账 / 任务高度（一条 SQL 都没有）。
func TestMinerGasFeesDryModeWritesDerivedTablesOnly(t *testing.T) {
	rp := buildSegReplay(t, false, newTraceAgg(segHeadEpoch), segReplayA, segReplayB)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Equal(t, 2, len(rp.inner.baseGasCosts))
	require.Equal(t, 2, len(rp.inner.methodGas))
	require.Equal(t, 2, len(rp.inner.minerGas))
	require.Empty(t, rp.inner.updateSectorGas, "trace-task 不改 base_gas_costs 的扇区费")

	// base_gas_costs：messages = 该高度 traces 数；acc_messages 因无上一条 = messages
	for i, rows := range rp.inner.baseGasCosts {
		wantEpoch := []int64{segReplayA, segReplayB}[i]
		require.Len(t, rows, 1)
		require.Equal(t, wantEpoch, rows[0].Epoch.Int64())
		require.Equal(t, int64(2), rows[0].Messages)
		require.Equal(t, int64(2), rows[0].AccMessages)
	}

	// method_gas_fees：两个方法各一行，epoch 必须就是清单高度
	for i, rows := range rp.inner.methodGas {
		wantEpoch := []int64{segReplayA, segReplayB}[i]
		require.Len(t, rows, 2)
		methods := map[string]bool{}
		for _, r := range rows {
			require.Equal(t, wantEpoch, r.Epoch)
			methods[r.Method] = true
		}
		require.True(t, methods["PreCommitSector"])
		require.True(t, methods["SubmitWindowedPoSt"])
	}

	// miner_gas_fees：SealGas = PreAgg + ProveAgg + SectorGas；WdPostGas 来自 traces
	for i, rows := range rp.inner.minerGas {
		wantEpoch := []int64{segReplayA, segReplayB}[i]
		require.Len(t, rows, 1)
		require.Equal(t, wantEpoch, rows[0].Epoch)
		require.Equal(t, "t0100", rows[0].Miner)
		require.Equal(t, 0, rows[0].PreAgg.Cmp(decimal.NewFromInt(11)))
		require.Equal(t, 0, rows[0].ProveAgg.Cmp(decimal.NewFromInt(22)))
		require.Equal(t, 0, rows[0].SectorGas.Cmp(decimal.NewFromInt(33)))
		require.Equal(t, 0, rows[0].SealGas.Cmp(decimal.NewFromInt(66)))
		require.Equal(t, 0, rows[0].WdPostGas.Cmp(decimal.NewFromInt(20)), "SubmitWindowedPoSt 的 GasCost 进 wd_post_gas")
	}

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// chain.base_gas_costs.acc_messages 是累计列：查得到上一条时接着累计（prev 不在空洞里也必须能查到）。
// 查不到（如只补空洞中段）时从零开始 —— 这正是必须按升序连续补的原因。
func TestMinerGasFeesAccumulatesAccMessagesFromPreviousRow(t *testing.T) {
	rp := buildSegReplay(t, false, newTraceAgg(segHeadEpoch), segReplayA)
	rp.inner.lastBaseGas = &stat.BaseGasCost{Epoch: chain.Epoch(segReplayA - 1), Messages: 9, AccMessages: 100}
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.baseGasCosts, 1)
	got := rp.inner.baseGasCosts[0][0]
	require.Equal(t, int64(2), got.Messages)
	require.Equal(t, int64(102), got.AccMessages, "acc_messages = 本高度 2 + 上一条 100")
}

// 聚合器取不到该高度（pre/pro/sector 三项全空）：miner_gas_fees 仍然有一行（来自 traces 的 PoSt），
// 但 pre_agg / prove_agg / sector_gas / seal_gas 全是 0 —— 这就是「输入缺失导致列偏低」的样子。
func TestMinerGasFeesMissingAggregatorDataYieldsZeroColumns(t *testing.T) {
	agg := newTraceAgg(segHeadEpoch)
	agg.preEmpty = true
	rp := buildSegReplay(t, false, agg, segReplayA)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.minerGas, 1)
	row := rp.inner.minerGas[0][0]
	require.True(t, row.PreAgg.IsZero())
	require.True(t, row.ProveAgg.IsZero())
	require.True(t, row.SectorGas.IsZero())
	require.True(t, row.SealGas.IsZero(), "seal_gas 会跟着偏低")
	require.Equal(t, 0, row.WdPostGas.Cmp(decimal.NewFromInt(20)), "wd_post_gas 只依赖 traces ⇒ 不受影响")
}

// 该高度没有 traces：trace-task 报 `traces is emtpy` ⇒ 该高度按保险丝重试后放弃，其余高度照跑，
// Execute 不把整批判失败（报告里逐条列出被放弃的高度）。
func TestMinerGasFeesEmptyTracesAbandonsOnlyThatHeight(t *testing.T) {
	const bad = segReplayA + 2 // 与两个健康高度都不同（清单去重后是 3 个高度）
	agg := &fakeTraceAgg{FakeAgg: offlinereplay.NewFakeAgg(segHeadEpoch, func(epoch chain.Epoch) []*londobell.TraceMessage {
		if epoch.Int64() == bad {
			return nil
		}
		return traces(epoch)
	})}
	rp := buildSegReplay(t, false, agg, segReplayA, bad, segReplayB)

	report := rp.execute(t, offlinereplay.RunOptions{EpochFailLimit: 2})

	require.Equal(t, 2, len(rp.inner.baseGasCosts), "只有两个健康高度补上了 chain.base_gas_costs")
	var patched []int64
	for _, rows := range rp.inner.baseGasCosts {
		patched = append(patched, rows[0].Epoch.Int64())
	}
	require.Equal(t, []int64{segReplayA, segReplayB}, patched)

	require.Empty(t, rp.rec.Statements(), "报错也不得写指针/台账/任务高度")
	require.Contains(t, report, "逐高度结果     : 已跑 3/3 个高度；放弃 1 个；未跑 0 个")
	require.Contains(t, report, fmt.Sprintf("- 高度 %d 失败 2 次", bad))
	require.Contains(t, report, "traces is emtpy")
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestMinerGasFeesRejectsHeightsAboveHead(t *testing.T) {
	rp := buildSegReplay(t, false, newTraceAgg(segHeadEpoch), segReplayA, segHeadEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", segHeadEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}
