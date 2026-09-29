package minerownercmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task/luck"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

// ---- 假依赖 ----
//
// 计算器的输入全部是库表查询（win counts / rewards / gas fees / miner_infos），
// 离线的假连接无法应答那些 SQL，所以这里提供三组假实现：
//   - fakeMinerTask     : 仓储（读返回预设数据；写全部记账，用于断言「写了什么」/「一次都没写」）
//   - fakeSyncerGetter  : 同步器进度查询（计算器先确认 chain 同步器已越过当前高度）
//   - fakeLuckRepo      : 幸运值仓储（本用例返回空集合，幸运值一律 0）
//
// 未在本文件实现的接口方法由嵌入接口兜底 —— 一旦被调用即 panic（用例在没走到的分支上不允许假绿）。

type fakeMinerTask struct {
	repository.MinerTask

	// 读侧预设：infos 对任何高度都返回同一批（变化量恒为 0）
	infos     []*po.MinerInfo
	rewards   []*bo.AccReward
	winCounts []*bo.AccWinCount
	gasFees   []*bo.AccGasFee
	infosErr  error

	mu              sync.Mutex
	infosCalls      int
	infosEpochs     []int64
	epochRows       []*po.SyncMinerEpochPo
	minerStats      [][]*po.MinerStat
	ownerStats      [][]*po.OwnerStat
	minerInfoWrites [][]*po.MinerInfo
	ownerInfoWrites [][]*po.OwnerInfo
	absPowerWrites  int
	deleteCalls     int
}

func (f *fakeMinerTask) GetMinerInfosByEpoch(_ context.Context, epoch chain.Epoch) ([]*po.MinerInfo, error) {
	f.mu.Lock()
	f.infosCalls++
	f.infosEpochs = append(f.infosEpochs, epoch.Int64())
	err := f.infosErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return f.infos, nil
}

func (f *fakeMinerTask) GetMinersAccRewards(_ context.Context, _ chain.LORCRange) ([]*bo.AccReward, error) {
	return f.rewards, nil
}

func (f *fakeMinerTask) GetMinersAccWinCount(_ context.Context, _ chain.LORCRange) ([]*bo.AccWinCount, error) {
	return f.winCounts, nil
}

func (f *fakeMinerTask) GetMinersAccGasFees(_ context.Context, _ chain.LORCRange) ([]*bo.AccGasFee, error) {
	return f.gasFees, nil
}

func (f *fakeMinerTask) SaveSyncMinerEpochPo(_ context.Context, item *po.SyncMinerEpochPo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.epochRows = append(f.epochRows, item)
	return nil
}

func (f *fakeMinerTask) SaveMinerStats(_ context.Context, stats []*po.MinerStat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minerStats = append(f.minerStats, stats)
	return nil
}

func (f *fakeMinerTask) SaveOwnerStats(_ context.Context, stats []*po.OwnerStat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ownerStats = append(f.ownerStats, stats)
	return nil
}

func (f *fakeMinerTask) SaveMinerInfos(_ context.Context, infos []*po.MinerInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minerInfoWrites = append(f.minerInfoWrites, infos)
	return nil
}

func (f *fakeMinerTask) SaveOwnerInfos(_ context.Context, infos []*po.OwnerInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ownerInfoWrites = append(f.ownerInfoWrites, infos)
	return nil
}

func (f *fakeMinerTask) SaveAbsPower(_ context.Context, _, _ decimal.Decimal, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.absPowerWrites++
	return nil
}

func (f *fakeMinerTask) DeleteSyncMinerEpochs(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteMinerStats(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteOwnerStats(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteMinerStatsBeforeEpoch(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteOwnerStatsBeforeEpoch(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteMinerInfos(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteOwnerInfos(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) DeleteAbsPower(_ context.Context, _ chain.Epoch) error {
	f.countDelete()
	return nil
}

func (f *fakeMinerTask) countDelete() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeMinerTask) Writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.epochRows) + len(f.minerStats) + len(f.ownerStats) +
		len(f.minerInfoWrites) + len(f.ownerInfoWrites) + f.absPowerWrites + f.deleteCalls
}

// LedgerEpochs 已写入 chain.sync_miner_epochs 的高度（按写入顺序）
func (f *fakeMinerTask) LedgerEpochs() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int64, 0, len(f.epochRows))
	for _, row := range f.epochRows {
		out = append(out, row.Epoch)
	}
	return out
}

// MinerStatRows / OwnerStatRows 已写入的统计行数
func (f *fakeMinerTask) MinerStatRows() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, batch := range f.minerStats {
		n += len(batch)
	}
	return n
}

func (f *fakeMinerTask) OwnerStatRows() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, batch := range f.ownerStats {
		n += len(batch)
	}
	return n
}

func (f *fakeMinerTask) StatCalls() (minerCalls, ownerCalls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.minerStats), len(f.ownerStats)
}

// MinerStatBatches / OwnerStatBatches 已写入的统计批次（按写入顺序的快照副本）
func (f *fakeMinerTask) MinerStatBatches() [][]*po.MinerStat {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]*po.MinerStat(nil), f.minerStats...)
}

func (f *fakeMinerTask) OwnerStatBatches() [][]*po.OwnerStat {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]*po.OwnerStat(nil), f.ownerStats...)
}

// LedgerRows chain.sync_miner_epochs 已写入的台账行（快照副本）
func (f *fakeMinerTask) LedgerRows() []*po.SyncMinerEpochPo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*po.SyncMinerEpochPo(nil), f.epochRows...)
}

func (f *fakeMinerTask) InfosCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.infosCalls
}

// InfosEpochs 被查询过 miner_infos 的高度（按查询顺序）
func (f *fakeMinerTask) InfosEpochs() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.infosEpochs...)
}

type fakeSyncerGetter struct {
	repository.SyncerGetter

	chainEpoch int64
}

// GetSyncer 计算器只用它确认 chain 同步器已越过当前高度；问其它同步器即失败（用例不允许假绿）
func (f *fakeSyncerGetter) GetSyncer(_ context.Context, name string) (*po.SyncSyncer, error) {
	if name != filscansyncer.ChainSyncer {
		return nil, fmt.Errorf("意外查询了同步器 %q：本命令的依赖只有 chain 同步器进度", name)
	}
	return &po.SyncSyncer{Name: name, Epoch: f.chainEpoch}, nil
}

type fakeLuckRepo struct {
	repository.LuckRepository

	miners []string
}

func (f *fakeLuckRepo) GetNetTickets(_ context.Context, _ chain.LCRORange, _ int64) ([]*bo.LuckNetTicket, error) {
	return nil, nil
}

func (f *fakeLuckRepo) GetNetTicketsYear(_ context.Context, _ chain.LCRORange, _ int64) ([]*bo.LuckNetTicket, error) {
	return nil, nil
}

func (f *fakeLuckRepo) GetNetQualityAjdPowerByPoints(_ context.Context, _ []chain.Epoch) ([]*bo.LuckQualityAdjPower, error) {
	return nil, nil
}

func (f *fakeLuckRepo) GetMinerQualityAjdPowerByPoints(_ context.Context, _ string, _ []chain.Epoch) ([]*bo.LuckQualityAdjPower, error) {
	return nil, nil
}

func (f *fakeLuckRepo) GetMiners(_ context.Context, _ int64) ([]string, error) {
	return f.miners, nil
}

func (f *fakeLuckRepo) GetMinerTicketsByEpochs(_ context.Context, _ string, _ chain.LCRORange) (int64, error) {
	return 0, nil
}

// swapDeps 把 buildTarget 用的依赖构造器换成假实现（生产路径始终是 dal 的三个构造器）
func swapDeps(inner *fakeMinerTask, sg repository.SyncerGetter, luckCalc *luck.Calculator) func() {
	old := newMinerDeps
	newMinerDeps = func(*gorm.DB) minerDeps {
		return minerDeps{repo: inner, sg: sg, luck: luckCalc}
	}
	return func() { newMinerDeps = old }
}

// ---- 参数校验：区间 / 清单 二选一，互斥 ----

func TestResolvePlanWiring(t *testing.T) {
	listPath := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(listPath, []byte("6408840\n6408960\n"), 0o600))

	t.Run("清单模式", func(t *testing.T) {
		plan, err := resolvePlan(options{epochsFile: listPath})
		require.NoError(t, err)
		require.True(t, plan.IsList())
		require.Equal(t, []int64{6408840, 6408960}, plan.List.Epochs())
	})

	t.Run("区间模式（左闭右闭）", func(t *testing.T) {
		plan, err := resolvePlan(options{start: 6408840, end: 6408960})
		require.NoError(t, err)
		require.False(t, plan.IsList())
		require.Equal(t, int64(6408840), plan.Range.From.Int64())
		require.Equal(t, int64(6408960), plan.Range.To.Int64())
		require.Equal(t, int64(121), plan.Count())
	})

	t.Run("--epochs-file 与 --start/--end 同时给出 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{start: 6408840, end: 6408960, epochsFile: listPath})
		require.Error(t, err)
		require.Contains(t, err.Error(), "互斥")
	})

	t.Run("两边都不给 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--epochs-file")
	})

	t.Run("只给一半 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{start: 6408840})
		require.Error(t, err)
		require.Contains(t, err.Error(), "截止高度")
	})

	t.Run("截止高度小于起始高度 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{start: 6408960, end: 6408840})
		require.Error(t, err)
		require.Contains(t, err.Error(), "不能小于")
	})

	t.Run("清单文件不存在 ⇒ 报错（不连任何生产依赖）", func(t *testing.T) {
		_, err := resolvePlan(options{epochsFile: filepath.Join(t.TempDir(), "nope.list")})
		require.Error(t, err)
		require.Contains(t, err.Error(), "读取高度清单")
	})
}

// 命令行契约：三种计划形态的 flag 都在；start/end 不是必填 flag（清单模式下不传它们），
// 互斥由 resolvePlan/ResolvePlan 判定。
func TestCommandFlags(t *testing.T) {
	cmd := Command()
	require.Equal(t, "calc-miner-owner", cmd.Name())

	require.NotNil(t, cmd.Flags().Lookup("config"))
	require.NotNil(t, cmd.Flags().Lookup("start"))
	require.NotNil(t, cmd.Flags().Lookup("end"))
	require.NotNil(t, cmd.Flags().Lookup("epochs-file"))
	require.NotNil(t, cmd.Flags().Lookup("no-write"))

	required := cobra.BashCompOneRequiredFlag
	require.NotNil(t, cmd.Flags().Lookup("config").Annotations[required], "config 必填")
	require.Nil(t, cmd.Flags().Lookup("start").Annotations[required], "start 不应是必填（清单模式不传）")
	require.Nil(t, cmd.Flags().Lookup("end").Annotations[required], "end 不应是必填（清单模式不传）")
}

// ---- 目标装配 ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeMinerTask{}
	restore := swapDeps(inner, &fakeSyncerGetter{}, luck.NewCalculator(&fakeLuckRepo{}))
	defer restore()

	t.Run("真写：只注册计算器 calc-miner-owner-task，且与生产一致不注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&config.Config{}, &gorm.DB{}, false)
		require.Equal(t, filscansyncer.MinerSyncer, target.Name, "与生产 Miner 同步器同名")
		require.Empty(t, target.Groups, "不注册 miner-task：chain.miner_infos 已就位，不重写")
		require.Len(t, target.Calculators, 1)
		require.Equal(t, "calc-miner-owner-task", target.Calculators[0].Name())
		require.True(t, target.SkipTraces, "本计算器不读 traces（全包无 Trace/Datamap 引用）")
		require.Nil(t, writes, "真写模式没有写统计来源（写入直接下发）")
	})

	t.Run("--no-write：仓储被「只统计不落库」包装替换", func(t *testing.T) {
		target, writes := buildTarget(&config.Config{}, &gorm.DB{}, true)
		require.Len(t, target.Calculators, 1)
		require.Equal(t, "calc-miner-owner-task", target.Calculators[0].Name())
		require.NotNil(t, writes)
	})
}

// ---- 「只统计不落库」包装 ----

func TestNoWriteMinerTaskRepoBlocksEveryWrite(t *testing.T) {
	inner := &fakeMinerTask{}
	var repo repository.MinerTask = inner
	wrapped := NewNoWriteMinerTaskRepo(repo)
	ctx := context.Background()

	// 计算器会走到的三个写方法 + 六个删除路径 + miner-task 的写方法：全部被拦下，一次都不转发
	require.NoError(t, wrapped.SaveSyncMinerEpochPo(ctx, &po.SyncMinerEpochPo{Epoch: 6408840, EffectiveMiners: 2, Owners: 1}))
	require.NoError(t, wrapped.SaveMinerStats(ctx, []*po.MinerStat{{Epoch: 6408840, Miner: "f0100"}, {Epoch: 6408840, Miner: "f0101"}}))
	require.NoError(t, wrapped.SaveOwnerStats(ctx, []*po.OwnerStat{{Epoch: 6408840, Owner: "f0999"}}))
	require.NoError(t, wrapped.SaveMinerInfos(ctx, []*po.MinerInfo{{Epoch: 6408840, Miner: "f0100"}}))
	require.NoError(t, wrapped.SaveOwnerInfos(ctx, []*po.OwnerInfo{{Epoch: 6408840, Owner: "f0999"}}))
	require.NoError(t, wrapped.SaveAbsPower(ctx, decimal.NewFromInt(1), decimal.NewFromInt(2), 6408840))
	require.NoError(t, wrapped.DeleteSyncMinerEpochs(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteMinerStats(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteOwnerStats(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteMinerStatsBeforeEpoch(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteOwnerStatsBeforeEpoch(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteMinerInfos(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteOwnerInfos(ctx, chain.Epoch(6408840)))
	require.NoError(t, wrapped.DeleteAbsPower(ctx, chain.Epoch(6408840)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	require.Equal(t, offlinereplay.WriteStat{Table: TableSyncMinerEpochs, Calls: 1, Rows: 1, Deletes: 1}, stats[TableSyncMinerEpochs])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerStats, Calls: 1, Rows: 2, Deletes: 2}, stats[TableMinerStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerStats, Calls: 1, Rows: 1, Deletes: 2}, stats[TableOwnerStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerInfos, Calls: 1, Rows: 1, Deletes: 1}, stats[TableMinerInfos])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerInfos, Calls: 1, Rows: 1, Deletes: 1}, stats[TableOwnerInfos])
	require.Equal(t, offlinereplay.WriteStat{Table: TableAbsPowerChange, Calls: 1, Rows: 1, Deletes: 1}, stats[TableAbsPowerChange])

	// 读方法原样透传（包装只拦写）
	inner.infos = []*po.MinerInfo{{Epoch: 6408840, Miner: "f0100"}}
	got, err := wrapped.GetMinerInfosByEpoch(ctx, chain.Epoch(6408840))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 1, inner.InfosCalls())
}

func TestNewNoWriteMinerTaskRepoRejectsNil(t *testing.T) {
	require.Panics(t, func() { NewNoWriteMinerTaskRepo(nil) })
}

var errFakeInfos = errors.New("fake: 该高度的 miner_infos 查询失败")
