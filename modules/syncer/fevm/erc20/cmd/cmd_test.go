package erc20cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

// fakeERC20Repo 假仓储：记录底层被写了几次。
// 它代表 fevm.erc_20_transfers / fevm.erc20_balance / fevm.erc20_swap_info / fevm.erc20_contract 的落库通道。
type fakeERC20Repo struct {
	// 嵌入接口：只实现离线回放会碰到的读写方法，其余被调用即 panic
	repository.ERC20TokenRepo

	writes int
}

func (f *fakeERC20Repo) CreateERC20TransferBatch(_ context.Context, _ []*po.FEvmERC20Transfer) error {
	f.writes++
	return nil
}

func (f *fakeERC20Repo) UpsertERC20BalanceBatch(_ context.Context, _ []*po.FEvmERC20Balance) error {
	f.writes++
	return nil
}

func (f *fakeERC20Repo) CreateERC20SwapInfoBatch(_ context.Context, _ []*po.FEvmERC20SwapInfo) error {
	f.writes++
	return nil
}

func (f *fakeERC20Repo) UpdateOneERC20Contract(_ context.Context, _ string, _ *po.FEvmERC20Contract) error {
	f.writes++
	return nil
}

func (f *fakeERC20Repo) CleanERC20TransferBatch(_ context.Context, _ int) error {
	f.writes++
	return nil
}

func (f *fakeERC20Repo) CleanERC20SwapInfo(_ context.Context, _ int) error {
	f.writes++
	return nil
}

// NewERC20Task 构造时会读一次方法签名表
func (f *fakeERC20Repo) GetAllMethodsDecodeSignature(_ context.Context) ([]po.FEvmMethods, error) {
	return nil, nil
}

type fakeABIDecoder struct{ fevm.ABIDecoderAPI }

func TestCommandRegistrationAndFlags(t *testing.T) {
	cmd := Command()
	require.Equal(t, "erc20", cmd.Name())

	for _, name := range []string{"config", "start", "end"} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "缺少参数 %s", name)
		require.Equal(t, []string{"true"}, flag.Annotations[cobra.BashCompOneRequiredFlag],
			"参数 %s 必须标记为必填", name)
	}
	noWrite := cmd.Flags().Lookup("no-write")
	require.NotNil(t, noWrite)
	require.Equal(t, "false", noWrite.DefValue, "--no-write 默认必须关闭")

	for _, want := range []string{"fevm.erc_20_transfers", "fevm.erc20_balance", "fevm.erc20_swap_info",
		"chain.sync_syncers", "chain.sync_task_epochs", "chain.sync_skipped_epochs", "--no-write"} {
		require.True(t, strings.Contains(cmd.Long, want), "Long 帮助应说明 %s", want)
	}

	cmd.SetArgs(nil)
	require.Error(t, cmd.Execute(), "缺必填参数必须在连依赖之前失败")
}

func TestNoWriteERC20RepoBlocksAllWrites(t *testing.T) {
	inner := &fakeERC20Repo{}
	wrapped := NewNoWriteERC20Repo(inner)

	ctx := context.Background()
	require.NoError(t, wrapped.CreateERC20TransferBatch(ctx, []*po.FEvmERC20Transfer{{}, {}}))
	require.NoError(t, wrapped.UpsertERC20BalanceBatch(ctx, []*po.FEvmERC20Balance{{}}))
	require.NoError(t, wrapped.CreateERC20SwapInfoBatch(ctx, []*po.FEvmERC20SwapInfo{{}, {}, {}}))
	require.NoError(t, wrapped.UpdateOneERC20Contract(ctx, "t1", &po.FEvmERC20Contract{}))
	require.NoError(t, wrapped.CleanERC20TransferBatch(ctx, 100))
	require.NoError(t, wrapped.CleanERC20SwapInfo(ctx, 100))

	require.Zero(t, inner.writes, "底层仓储一次写都不该被调用")

	stats := wrapped.WriteStats()
	require.Len(t, stats, 4)
	byTable := map[string]offlinereplay.WriteStat{}
	for _, s := range stats {
		byTable[s.Table] = s
	}
	require.Equal(t, int64(1), byTable[TableERC20Transfers].Calls)
	require.Equal(t, int64(2), byTable[TableERC20Transfers].Rows)
	require.Equal(t, int64(1), byTable[TableERC20Transfers].Deletes, "CleanERC20TransferBatch 是删除路径")
	require.Equal(t, int64(1), byTable[TableERC20Balance].Rows)
	require.Equal(t, int64(3), byTable[TableERC20SwapInfo].Rows)
	require.Equal(t, int64(1), byTable[TableERC20SwapInfo].Deletes)
	require.Equal(t, int64(1), byTable[TableERC20Contract].Rows)

	require.Panics(t, func() { NewNoWriteERC20Repo(nil) })
}

// 派生表名常量必须与 po 的 TableName 一致
func TestTableNameConstants(t *testing.T) {
	require.Equal(t, po.FEvmERC20Transfer{}.TableName(), TableERC20Transfers)
	require.Equal(t, po.FEvmERC20Balance{}.TableName(), TableERC20Balance)
	require.Equal(t, po.FEvmERC20SwapInfo{}.TableName(), TableERC20SwapInfo)
	require.Equal(t, po.FEvmERC20Contract{}.TableName(), TableERC20Contract)
}

// buildTarget：同步器名与生产一致、任务为 erc20-task；--no-write 时写统计来源非 nil 且全为 0。
// 这里把仓储构造换成假仓储（生产路径 erc20.NewERC20Task 构造时会读方法签名表，离线假连接无法应答）。
func TestBuildTargetWiring(t *testing.T) {
	db := offlinereplay.OpenTestDB(t, &offlinereplay.Recorder{})
	decoder := &fakeABIDecoder{}

	inner := &fakeERC20Repo{}
	orig := newERC20Repo
	newERC20Repo = func(*gorm.DB) repository.ERC20TokenRepo { return inner }
	defer func() { newERC20Repo = orig }()

	t.Run("真写模式不注入计数包装", func(t *testing.T) {
		target, writes := buildTarget(db, decoder, nil, false)
		require.Equal(t, "erc20", target.Name)
		require.Equal(t, []string{"erc20-task"}, target.TaskNames())
		require.Empty(t, target.Calculators, "生产 Erc20Syncer 没有计算器")
		require.Nil(t, writes, "真写模式不计数（写入直接下发）")
	})

	t.Run("no-write 模式注入计数包装", func(t *testing.T) {
		target, writes := buildTarget(db, decoder, nil, true)
		require.Equal(t, "erc20", target.Name)
		require.Equal(t, []string{"erc20-task"}, target.TaskNames())
		require.NotNil(t, writes)

		stats := writes()
		require.Len(t, stats, 4)
		for _, s := range stats {
			require.Zero(t, s.Calls, "%s 不该有写入", s.Table)
			require.Zero(t, s.Rows)
		}
	})
}
