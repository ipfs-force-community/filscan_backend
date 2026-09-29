package minergasfeecmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

// fakeTraceRepo 假仓储：记录写方法是否被调用（证明 --no-write 下「一次都没转发到真仓储」）。
// 只实现本包链路会走到的读写方法，其余由嵌入接口兜底（未实现的方法一旦被调用即 panic —— 不允许假绿）。
type fakeTraceRepo struct {
	repository.SyncerTraceTaskRepo

	baseGasCosts [][]*stat.BaseGasCost
	methodGas    [][]*po.MethodGasFee
	minerGas     [][]*po.MinerGasFee

	updateSectorGas []chain.Epoch
	deleteBaseGas   []chain.Epoch
	deleteMethodGas []chain.Epoch
	deleteMinerGas  []chain.Epoch

	lastBaseGas *stat.BaseGasCost
}

func (f *fakeTraceRepo) GetLastBaseGasCostOrNil(_ context.Context, _ chain.Epoch) (*stat.BaseGasCost, error) {
	return f.lastBaseGas, nil
}

func (f *fakeTraceRepo) SaveBaseGasCost(_ context.Context, base *stat.BaseGasCost) error {
	f.baseGasCosts = append(f.baseGasCosts, []*stat.BaseGasCost{base})
	return nil
}

func (f *fakeTraceRepo) UpdateBaseGasCostSectorGas(_ context.Context, e chain.Epoch, _, _ decimal.Decimal) error {
	f.updateSectorGas = append(f.updateSectorGas, e)
	return nil
}

func (f *fakeTraceRepo) SaveMethodGasFees(_ context.Context, entities []*po.MethodGasFee) error {
	f.methodGas = append(f.methodGas, entities)
	return nil
}

func (f *fakeTraceRepo) SaveMinerGasFees(_ context.Context, items []*po.MinerGasFee) error {
	f.minerGas = append(f.minerGas, items)
	return nil
}

func (f *fakeTraceRepo) DeleteBaseGasCosts(_ context.Context, e chain.Epoch) error {
	f.deleteBaseGas = append(f.deleteBaseGas, e)
	return nil
}

func (f *fakeTraceRepo) DeleteMethodGasFees(_ context.Context, e chain.Epoch) error {
	f.deleteMethodGas = append(f.deleteMethodGas, e)
	return nil
}

func (f *fakeTraceRepo) DeleteMinerGasFees(_ context.Context, e chain.Epoch) error {
	f.deleteMinerGas = append(f.deleteMinerGas, e)
	return nil
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeTraceRepo) Writes() int {
	return len(f.baseGasCosts) + len(f.methodGas) + len(f.minerGas) + len(f.updateSectorGas) +
		len(f.deleteBaseGas) + len(f.deleteMethodGas) + len(f.deleteMinerGas)
}

// fakeChangeActorRepo 只为 typer.MinerSectorSize 提供扇区大小（生产上它读 chain.sync_miner_epochs +
// chain.miner_infos，回放时通常已是链头那行；取不到才回退 adapter.Miner）。
type fakeChangeActorRepo struct {
	repository.ChangeActorTask
	size int64
}

func (f *fakeChangeActorRepo) GetMinerSizeOrZero(_ context.Context, _ string) (int64, error) {
	return f.size, nil
}

// ---- 参数校验：区间 / 清单 二选一，互斥 ----

func TestResolvePlanWiring(t *testing.T) {
	listPath := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(listPath, []byte("6357121\n6357123\n"), 0o600))

	t.Run("清单模式", func(t *testing.T) {
		plan, err := resolvePlan(options{epochsFile: listPath})
		require.NoError(t, err)
		require.True(t, plan.IsList())
		require.Equal(t, []int64{6357121, 6357123}, plan.List.Epochs())
	})

	t.Run("区间模式（左闭右闭）", func(t *testing.T) {
		plan, err := resolvePlan(options{start: 6357120, end: 6408648})
		require.NoError(t, err)
		require.False(t, plan.IsList())
		require.Equal(t, int64(6408648-6357120+1), plan.Count())
	})

	t.Run("--epochs-file 与 --start/--end 同时给出 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{start: 6357120, end: 6408648, epochsFile: listPath})
		require.Error(t, err)
		require.Contains(t, err.Error(), "互斥")
	})

	t.Run("两边都不给 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--epochs-file")
	})

	t.Run("清单文件不存在 ⇒ 报错（不连任何生产依赖）", func(t *testing.T) {
		_, err := resolvePlan(options{epochsFile: filepath.Join(t.TempDir(), "nope.list")})
		require.Error(t, err)
		require.Contains(t, err.Error(), "读取高度清单")
	})
}

// 命令行契约：四种 flag 都在；start/end 不是必填（清单模式不传）。
func TestCommandFlags(t *testing.T) {
	cmd := Command()

	require.NotNil(t, cmd.Flags().Lookup("config"))
	require.NotNil(t, cmd.Flags().Lookup("start"))
	require.NotNil(t, cmd.Flags().Lookup("end"))
	require.NotNil(t, cmd.Flags().Lookup("epochs-file"))
	require.NotNil(t, cmd.Flags().Lookup("no-write"))

	required := cobra.BashCompOneRequiredFlag
	require.NotNil(t, cmd.Flags().Lookup("config").Annotations[required], "config 仍必填")
	require.Nil(t, cmd.Flags().Lookup("start").Annotations[required], "start 不应是必填")
	require.Nil(t, cmd.Flags().Lookup("end").Annotations[required], "end 不应是必填")
	require.Equal(t, "miner-gas-fees", cmd.Name())
}

// ---- 目标装配：本任务读 traces ⇒ 必须注入 traces（SkipTraces 必须为 false） ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeTraceRepo{}
	restore := restoreTraceRepo(inner)
	defer restore()

	adapter := &offlinereplay.FakeAdapter{}

	t.Run("真写：同步器名与生产一致，只注册 trace-task，且注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, adapter, false)
		require.Equal(t, filscansyncer.ChainSyncer, target.Name)
		require.Len(t, target.Groups, 1)
		require.Len(t, target.Groups[0], 1)
		require.Equal(t, "trace-task", target.Groups[0][0].Name())
		require.Empty(t, target.Calculators, "本命令只跑 sync task，不跑 chain 同步器的计算器")
		require.False(t, target.SkipTraces, "trace-task 读 Datamap 里的 traces ⇒ 必须注入")
		require.Nil(t, writes, "真写模式没有写统计来源（写入直接下发）")
	})

	t.Run("--no-write：仓储被「只统计不落库」包装替换", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, adapter, true)
		require.Len(t, target.Groups[0], 1)
		require.False(t, target.SkipTraces)
		require.NotNil(t, writes)
	})
}

// ---- 「只统计不落库」包装 ----

func TestNoWriteTraceRepoBlocksEveryWrite(t *testing.T) {
	inner := &fakeTraceRepo{}
	var repo repository.SyncerTraceTaskRepo = inner
	wrapped := NewNoWriteTraceRepo(repo)
	ctx := context.Background()

	require.NoError(t, wrapped.SaveBaseGasCost(ctx, &stat.BaseGasCost{Epoch: 6357121}))
	require.NoError(t, wrapped.UpdateBaseGasCostSectorGas(ctx, chain.Epoch(6357121), decimal.NewFromInt(1), decimal.NewFromInt(2)))
	require.NoError(t, wrapped.SaveMethodGasFees(ctx, []*po.MethodGasFee{{Epoch: 6357121}, {Epoch: 6357121}}))
	require.NoError(t, wrapped.SaveMinerGasFees(ctx, []*po.MinerGasFee{{Epoch: 6357121}}))
	require.NoError(t, wrapped.DeleteBaseGasCosts(ctx, chain.Epoch(6357121)))
	require.NoError(t, wrapped.DeleteMethodGasFees(ctx, chain.Epoch(6357121)))
	require.NoError(t, wrapped.DeleteMinerGasFees(ctx, chain.Epoch(6357121)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	// base_gas_costs：写 2 次（一次 Save + 一次 Update，各 1 行）、删 1 次
	require.Equal(t, offlinereplay.WriteStat{Table: TableBaseGasCosts, Calls: 2, Rows: 2, Deletes: 1}, stats[TableBaseGasCosts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMethodGasFees, Calls: 1, Rows: 2, Deletes: 1}, stats[TableMethodGasFees])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerGasFees, Calls: 1, Rows: 1, Deletes: 1}, stats[TableMinerGasFees])

	// 读方法原样透传（包装只拦写）
	inner.lastBaseGas = &stat.BaseGasCost{Epoch: 6357000, AccMessages: 42}
	got, err := wrapped.GetLastBaseGasCostOrNil(ctx, chain.Epoch(6357121))
	require.NoError(t, err)
	require.Equal(t, int64(42), got.AccMessages)
}

func TestNewNoWriteTraceRepoRejectsNil(t *testing.T) {
	require.Panics(t, func() { NewNoWriteTraceRepo(nil) })
}

// restoreTraceRepo 把 buildTarget 用的仓储构造器换成假仓储（生产路径仍是 dal.NewSyncerTraceTaskDal）
func restoreTraceRepo(inner repository.SyncerTraceTaskRepo) func() {
	old := newTraceRepo
	newTraceRepo = func(*gorm.DB) repository.SyncerTraceTaskRepo { return inner }
	return func() { newTraceRepo = old }
}
