package minerownercmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task/luck"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

// 本文件用**真实的 CalcMinerOwnerTask** 跑完整条 Dry 管线，是「calc-miner-owner 子命令真能补
// chain.sync_miner_epochs 台账 + chain.miner_stats + chain.owner_stats、只在整点高度写、
// --no-write 下三张表零写入、清单/区间两种形态都能跑」的主证据。
//
// 高度选择：
//   - replayOn / replayOn2 都是 120 的整数倍（整点高度）且 %2880 != 2160
//     （避免走到日结与 1y 批次，那两条分支另有用途）；
//   - replayOff 是整点高度的下一个高度（%120 != 0）⇒ 计算器直接空跑（calc_miner_owner_task.go:85）。

const (
	headEpoch = 6409050 // 假聚合器链头（head-1 才是可回放的最高高度）
	replayOn  = 6408840 // 整点高度（6408840 % 120 == 0）
	replayOn2 = 6408960 // 另一个整点高度
	replayOff = 6408841 // 非整点高度（%120 != 0）—— 计算器空跑
)

// minerInfos 两个 Miner、同一个 Owner 的 MinerInfo（=> 每个区间写 2 行 miner_stats、1 行 owner_stats）
func minerInfos() []*po.MinerInfo {
	return []*po.MinerInfo{
		{
			Epoch:           0, // 假仓储对所有高度返回同一批，高度由计算器按目标高度建 stat
			Miner:           "f0100",
			Owner:           "f0999",
			RawBytePower:    decimal.NewFromInt(1000),
			QualityAdjPower: decimal.NewFromInt(1000),
			InitialPledge:   decimal.NewFromInt(10),
			SectorCount:     10,
			SectorSize:      34359738368,
		},
		{
			Epoch:           0,
			Miner:           "f0101",
			Owner:           "f0999",
			RawBytePower:    decimal.NewFromInt(2000),
			QualityAdjPower: decimal.NewFromInt(2000),
			InitialPledge:   decimal.NewFromInt(20),
			SectorCount:     20,
			SectorSize:      34359738368,
		},
	}
}

// minerRewards / minerWinCounts / minerGasFees 累计数据（非零，确保 AccRewardPercent 等百分比路径也被走到）
func minerRewards() []*bo.AccReward {
	return []*bo.AccReward{
		{Miner: "f0100", Reward: decimal.NewFromInt(100), BlockCount: 2},
		{Miner: "f0101", Reward: decimal.NewFromInt(300), BlockCount: 4},
	}
}

func minerWinCounts() []*bo.AccWinCount {
	return []*bo.AccWinCount{
		{Miner: "f0100", WinCount: 3},
		{Miner: "f0101", WinCount: 9},
	}
}

func minerGasFees() []*bo.AccGasFee {
	return []*bo.AccGasFee{
		{Miner: "f0100", SealGas: decimal.NewFromInt(5), WdPostGas: decimal.NewFromInt(7)},
		{Miner: "f0101", SealGas: decimal.NewFromInt(11), WdPostGas: decimal.NewFromInt(13)},
	}
}

// powerActorState 适配器返回的 PowerActor 状态：计算器只取 TotalQualityAdjPower
// （calc_miner_owner_task.go:169 GetNetQualityAdjPower → json → upgrader.UnmarshalerPowerState）
func powerActorState() *londobell.ActorState {
	return &londobell.ActorState{
		ActorID:   "f04",
		ActorAddr: "f04",
		ActorType: "power",
		State:     map[string]string{"TotalQualityAdjPower": "1000000"},
	}
}

// minerReplay 一次回放的装配结果
type minerReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *offlinereplay.FakeAgg
	adapter *offlinereplay.FakeAdapter
	inner   *fakeMinerTask
}

// buildListReplay 用真实计算器 + 假仓储/假聚合器/假适配器 + 假连接装配「高度清单」回放
func buildListReplay(t *testing.T, noWrite bool, heights ...int64) *minerReplay {
	t.Helper()
	text := ""
	for _, h := range heights {
		text += fmt.Sprintf("%d\n", h)
	}
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)
	return newMinerReplay(t, noWrite, plan)
}

// buildRangeReplay 同上的「连续区间」形态
func buildRangeReplay(t *testing.T, noWrite bool, from, to int64) *minerReplay {
	t.Helper()
	plan, err := offlinereplay.ResolvePlan(from, to, "")
	require.NoError(t, err)
	return newMinerReplay(t, noWrite, plan)
}

func newMinerReplay(t *testing.T, noWrite bool, plan offlinereplay.Plan) *minerReplay {
	t.Helper()

	inner := &fakeMinerTask{
		infos:     minerInfos(),
		rewards:   minerRewards(),
		winCounts: minerWinCounts(),
		gasFees:   minerGasFees(),
	}
	restore := swapDeps(inner, &fakeSyncerGetter{chainEpoch: headEpoch},
		luck.NewCalculator(&fakeLuckRepo{miners: []string{"f0100", "f0101"}}))
	t.Cleanup(restore)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)
	agg := offlinereplay.NewFakeAgg(headEpoch, nil)
	adapter := &offlinereplay.FakeAdapter{State: powerActorState()}

	// 生产同步器的配置形态：非测试网（=> 只有 %120 == 0 的整点高度才干活）
	conf := &config.Config{TestNet: false}
	target, writes := buildTarget(conf, db, noWrite)

	return &minerReplay{rec: rec, db: db, plan: plan, target: target, writes: writes,
		agg: agg, adapter: adapter, inner: inner}
}

// execute 跑一次回放并返回打印出来的报告
func (r *minerReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

// captureStdout 抓取 fn 期间写入 os.Stdout 的内容（Execute 用 fmt.Println 打印报告）
func captureStdout(t *testing.T, fn func()) string {
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

// writeStats 把报告用的写统计按表名索引
func (r *minerReplay) writeStats(t *testing.T) map[string]offlinereplay.WriteStat {
	t.Helper()
	require.NotNil(t, r.writes, "只有 --no-write 才有写统计来源")
	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range r.writes() {
		stats[s.Table] = s
	}
	return stats
}

// ---- 真写：台账/统计行被真实写入（假仓储），Dry 保证不写指针与台账 ----

func TestMinerOwnerListModeWritesLedgerAndStatsOnBoundaryHeights(t *testing.T) {
	rp := buildListReplay(t, false, replayOn, replayOn2)

	report := rp.execute(t, offlinereplay.RunOptions{})

	// ① chain.sync_miner_epochs：每个整点高度正好一行（页面「该整点算完了吗」的判据）
	rows := rp.inner.LedgerRows()
	require.Len(t, rows, 2, "两个整点高度各写一行台账")
	sort.Slice(rows, func(i, j int) bool { return rows[i].Epoch < rows[j].Epoch })
	require.Equal(t, []int64{replayOn, replayOn2}, []int64{rows[0].Epoch, rows[1].Epoch},
		"台账行的高度就是清单里的高度")
	for _, row := range rows {
		require.EqualValues(t, 2, row.EffectiveMiners, "有效 Miner 数 = minerStats 的行数")
		require.EqualValues(t, 1, row.Owners, "Owner 数 = ownerStats 的行数")
	}

	// ② chain.miner_stats：每高度每区间一批（2 个 Miner × 24h/7d/30d）
	minerBatches := rp.inner.MinerStatBatches()
	require.Len(t, minerBatches, 6, "两个高度 × 三个区间")
	perHeight := map[int64][]string{}
	for _, batch := range minerBatches {
		require.Len(t, batch, 2, "两个 Miner")
		for _, stat := range batch {
			perHeight[stat.Epoch] = append(perHeight[stat.Epoch], stat.Interval)
			require.Contains(t, []string{"f0100", "f0101"}, stat.Miner)
			require.True(t, stat.AccReward.GreaterThan(decimal.Zero), "累计奖励来自仓储查询")
			require.True(t, stat.QualityAdjPower().GreaterThan(decimal.Zero))
			require.Equal(t, stat.Epoch-prevOffset(stat.Interval), stat.PrevEpochRef,
				"PrevEpochRef = 该区间的起点高度（24h/7d/30d 分别回退 2880/7*2880/30*2880）")
		}
	}
	require.Equal(t, []int64{replayOn, replayOn2}, sortedKeys(perHeight))
	for _, intervals := range perHeight {
		sort.Strings(intervals)
		require.Equal(t, []string{"24h", "30d", "7d"}, dedup(intervals), "每个整点高度写 24h/7d/30d 三个区间")
	}
	require.Equal(t, 12, rp.inner.MinerStatRows())

	// ③ chain.owner_stats：两个 Miner 同属一个 Owner ⇒ 每区间一批、一批一行
	ownerBatches := rp.inner.OwnerStatBatches()
	require.Len(t, ownerBatches, 6)
	for _, batch := range ownerBatches {
		require.Len(t, batch, 1, "两个 Miner 同一个 Owner ⇒ 一个 Owner 一行")
		require.Equal(t, "f0999", batch[0].Owner)
		require.Contains(t, []int64{replayOn, replayOn2}, batch[0].Epoch)
	}
	require.Equal(t, 6, rp.inner.OwnerStatRows())

	// ④ Dry 模式：同步指针 / 同步器任务台账 / 跳过台账一条 SQL 都不写
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs")

	// ⑤ 本计算器不读 traces（SkipTraces）⇒ 一个高度一次 Traces 调用都不该有
	require.Empty(t, rp.agg.EpochsRequested(), "SkipTraces 生效：不注入 traces")
	// 每个整点高度只查一次 PowerActor 全网算力
	calls, errs := rp.adapter.Calls()
	require.Equal(t, 2, calls)
	require.Zero(t, errs)

	require.Contains(t, report, "同步器/任务     : miner / calc-miner-owner-task")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "模式           : 真写（直接写派生表；仍不写同步指针/台账/任务高度）")
}

// 非整点高度：计算器在 %120 判定处直接 return（连 MinerInfos 都不查、adapter 也不调），台账不写
func TestMinerOwnerRangeModeSkipsNonBoundaryEpoch(t *testing.T) {
	rp := buildRangeReplay(t, false, replayOn, replayOff)

	report := rp.execute(t, offlinereplay.RunOptions{})

	require.Equal(t, []int64{replayOn}, rp.inner.LedgerEpochs(),
		"只有整点高度 6408840 写台账；6408841 (%120 != 0) 不写")
	require.Equal(t, 4, rp.inner.InfosCalls(),
		"整点高度查 miner_infos 4 次（本高度 1 次 + 24h/7d/30d 各查一次前序高度）；非整点高度一次都不查")
	queried := rp.inner.InfosEpochs()
	require.NotContains(t, queried, replayOff, "非整点高度连一次库表查询都没有")
	sort.Slice(queried, func(i, j int) bool { return queried[i] < queried[j] })
	require.Equal(t, []int64{replayOn - 2880*30, replayOn - 2880*7, replayOn - 2880, replayOn}, queried,
		"被查询的高度恰为整点高度及其 24h/7d/30d 区间起点")
	calls, _ := rp.adapter.Calls()
	require.Equal(t, 1, calls, "非整点高度不查 PowerActor（在 GetNetQualityAdjPower 之前就 return 了）")

	require.Equal(t, 3, len(rp.inner.MinerStatBatches()), "只有整点高度写了三个区间")
	require.Equal(t, 3, len(rp.inner.OwnerStatBatches()))
	require.Empty(t, rp.rec.Statements())
	require.Contains(t, report, fmt.Sprintf("高度区间       : [%d, %d] 左闭右闭，共 2 个高度", replayOn, replayOff))
}

// ---- --no-write：三张表全被拦下，只计数不落库 ----

func TestMinerOwnerNoWriteCountsAllThreeTablesWithoutAnyWrite(t *testing.T) {
	rp := buildListReplay(t, true, replayOn, replayOn2)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(), "Dry 模式一条 SQL 都不该有")
	require.Equal(t, 8, rp.inner.InfosCalls(), "两个整点高度各查 4 次 miner_infos")
	acalls, _ := rp.adapter.Calls()
	require.Equal(t, 2, acalls, "两个整点高度各查一次 PowerActor")

	stats := rp.writeStats(t)
	// 每个整点高度：台账 1 次/1 行；miner_stats 3 次/6 行（2 Miner × 3 区间）；owner_stats 3 次/3 行（1 Owner × 3 区间）
	require.Equal(t, offlinereplay.WriteStat{Table: TableSyncMinerEpochs, Calls: 2, Rows: 2}, stats[TableSyncMinerEpochs])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerStats, Calls: 6, Rows: 12}, stats[TableMinerStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerStats, Calls: 6, Rows: 6}, stats[TableOwnerStats])
	// 本命令不注册 miner-task ⇒ 这三张表一次都不碰
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerInfos}, stats[TableMinerInfos])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerInfos}, stats[TableOwnerInfos])
	require.Equal(t, offlinereplay.WriteStat{Table: TableAbsPowerChange}, stats[TableAbsPowerChange])

	require.Contains(t, report, "模式           : --no-write 只统计不落库")
	require.Contains(t, report, "chain.sync_miner_epochs 写 2 次/2 行；删除 0 次")
	require.Contains(t, report, "chain.miner_stats 写 6 次/12 行；删除 0 次")
	require.Contains(t, report, "chain.owner_stats 写 6 次/6 行；删除 0 次")
	require.Contains(t, report, "被拦下的合计   : 写 14 次/20 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 20 行（本次未落库）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
}

// 非整点高度 + --no-write：三张表都零写入（既不真写、也没有可计数的写入）
func TestMinerOwnerNoWriteNonBoundaryEpochWritesNothing(t *testing.T) {
	rp := buildListReplay(t, true, replayOff)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes())
	require.Zero(t, rp.inner.InfosCalls(), "非整点高度不查任何库表")
	calls, _ := rp.adapter.Calls()
	require.Zero(t, calls, "非整点高度不查 PowerActor")
	// 报告说「已跑 1/1」⇒ 框架确实把这个非整点高度跑过了（不是被跳过），却一条写入都没有
	require.Contains(t, report, "逐高度结果     : 已跑 1/1 个高度；放弃 0 个；未跑 0 个")
	stats := rp.writeStats(t)
	for _, table := range []string{TableSyncMinerEpochs, TableMinerStats, TableOwnerStats,
		TableMinerInfos, TableOwnerInfos, TableAbsPowerChange} {
		require.Equal(t, offlinereplay.WriteStat{Table: table}, stats[table],
			"非整点高度 %d 不该有任何写入（表 %s）", replayOff, table)
	}
	require.Contains(t, report, "chain.sync_miner_epochs 写 0 次/0 行；删除 0 次")
	require.Contains(t, report, "被拦下的合计   : 写 0 次/0 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 0 行（本次未落库）")
}

// --no-write 也能跑连续区间形态（单高度区间：只跑这一个高度）
func TestMinerOwnerNoWriteRangeModeCountsAndBlocks(t *testing.T) {
	rp := buildRangeReplay(t, true, replayOn, replayOn)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements())

	stats := rp.writeStats(t)
	require.Equal(t, offlinereplay.WriteStat{Table: TableSyncMinerEpochs, Calls: 1, Rows: 1}, stats[TableSyncMinerEpochs])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerStats, Calls: 3, Rows: 6}, stats[TableMinerStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerStats, Calls: 3, Rows: 3}, stats[TableOwnerStats])

	require.Contains(t, report, fmt.Sprintf("高度区间       : [%d, %d] 左闭右闭，共 1 个高度", replayOn, replayOn))
	require.Contains(t, report, "chain.sync_miner_epochs 写 1 次/1 行；删除 0 次")
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestMinerOwnerRejectsHeightsAboveHead(t *testing.T) {
	rp := buildListReplay(t, true, replayOn, headEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)

	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", headEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}

// prevOffset 各统计区间的回退高度（与 calc_miner_owner_task.go 的 prevEpoch 计算一致）
func prevOffset(interval string) int64 {
	switch interval {
	case "24h":
		return 2880
	case "7d":
		return 2880 * 7
	case "30d":
		return 2880 * 30
	}
	return 0
}

// sortedKeys map[int64][]string 的升序键
func sortedKeys(m map[int64][]string) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// dedup 去重（保持入参已排序时的顺序稳定；interval 的取值来自 map 遍历，故先排序再进这里）
func dedup(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
