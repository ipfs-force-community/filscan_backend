package fnscmd

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
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// fakeFnsSaver / fakeFEvmRepo：记录底层被写了几次，代表 fns.* 与 fevm.* 的落库通道。
type fakeFnsSaver struct {
	repository.FnsSaver

	writes int
}

func (f *fakeFnsSaver) AddEvents(_ context.Context, items []*po.FNSEvent) error {
	f.writes += len(items)
	return nil
}

func (f *fakeFnsSaver) AddToken(_ context.Context, item ...*po.FNSToken) error {
	f.writes += len(item)
	return nil
}

func (f *fakeFnsSaver) AddAction(_ context.Context, _ *po.FNSAction) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) AddTransfer(_ context.Context, _ *po.FNSTransfer) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) AddFNsReserveDomain(_ context.Context, _ *po.FnsReserve) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) AddFnsReserveDomainWithConflict(_ context.Context, _ *po.FnsReserve) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) DeleteTokenByName(_ context.Context, _, _ string) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) DeleteEventsAfterEpoch(_ context.Context, _ chain.Epoch) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) DeleteTransferAfterEpoch(_ context.Context, _ chain.Epoch) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) DeleteActionsAfterEpoch(_ context.Context, _ chain.Epoch) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) DeleteOriginReserve(_ context.Context, _, _ string) error {
	f.writes++
	return nil
}

func (f *fakeFnsSaver) DeleteFnsReservesAfterEpoch(_ context.Context, _ chain.Epoch) error {
	f.writes++
	return nil
}

type fakeFEvmRepo struct {
	repository.FEvmRepo

	writes int
}

func (f *fakeFEvmRepo) CreateERC20TransferBatch(_ context.Context, items []*po.FEvmERC20Transfer) error {
	f.writes += len(items)
	return nil
}

func (f *fakeFEvmRepo) CreateErc721TransferBatch(_ context.Context, items []*po.NFTTransfer) error {
	f.writes += len(items)
	return nil
}

func (f *fakeFEvmRepo) CreateErc721Tokens(_ context.Context, items []*po.NFTToken) error {
	f.writes += len(items)
	return nil
}

func (f *fakeFEvmRepo) SaveAPISignatures(_ context.Context, items []*po.FEvmABISignature) error {
	f.writes += len(items)
	return nil
}

type fakeABIDecoder struct{ fevm.ABIDecoderAPI }

func TestCommandRegistrationAndFlags(t *testing.T) {
	cmd := Command()
	require.Equal(t, "fns", cmd.Name())

	for _, name := range []string{"config", "start", "end"} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "缺少参数 %s", name)
		require.Equal(t, []string{"true"}, flag.Annotations[cobra.BashCompOneRequiredFlag],
			"参数 %s 必须标记为必填", name)
	}

	for _, name := range []string{"no-write", "skip-calculator"} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "缺少参数 --%s", name)
		require.Equal(t, "false", flag.DefValue, "--%s 默认必须关闭", name)
		require.Empty(t, flag.Annotations[cobra.BashCompOneRequiredFlag], "--%s 必须可选", name)
	}

	for _, want := range []string{"fns.events", "fns.tokens", "fns.actions", "fns.transfers", "fns.reverses",
		"chain.sync_syncers", "chain.sync_task_epochs", "chain.sync_skipped_epochs", "--no-write", "--skip-calculator"} {
		require.True(t, strings.Contains(cmd.Long, want), "Long 帮助应说明 %s", want)
	}

	cmd.SetArgs(nil)
	require.Error(t, cmd.Execute(), "缺必填参数必须在连依赖之前失败")
}

func TestNoWriteFnsSaverBlocksAllWrites(t *testing.T) {
	inner := &fakeFnsSaver{}
	wrapped := NewNoWriteFnsSaver(inner)

	ctx := context.Background()
	require.NoError(t, wrapped.AddEvents(ctx, []*po.FNSEvent{{}, {}}))
	require.NoError(t, wrapped.AddToken(ctx, &po.FNSToken{}, &po.FNSToken{}))
	require.NoError(t, wrapped.AddAction(ctx, &po.FNSAction{}))
	require.NoError(t, wrapped.AddTransfer(ctx, &po.FNSTransfer{}))
	require.NoError(t, wrapped.AddFNsReserveDomain(ctx, &po.FnsReserve{}))
	require.NoError(t, wrapped.AddFnsReserveDomainWithConflict(ctx, &po.FnsReserve{}))
	require.NoError(t, wrapped.DeleteTokenByName(ctx, "a.fil", "opengate"))
	require.NoError(t, wrapped.DeleteEventsAfterEpoch(ctx, chain.Epoch(10)))
	require.NoError(t, wrapped.DeleteTransferAfterEpoch(ctx, chain.Epoch(10)))
	require.NoError(t, wrapped.DeleteActionsAfterEpoch(ctx, chain.Epoch(10)))
	require.NoError(t, wrapped.DeleteOriginReserve(ctx, "t1", "a.fil"))
	require.NoError(t, wrapped.DeleteFnsReservesAfterEpoch(ctx, chain.Epoch(10)))

	require.Zero(t, inner.writes, "底层仓储一次写都不该被调用")

	byTable := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		byTable[s.Table] = s
	}
	require.Len(t, byTable, 5)
	require.Equal(t, int64(2), byTable[TableFNSEvents].Rows, "AddEvents 2 行")
	require.Equal(t, int64(1), byTable[TableFNSEvents].Deletes, "DeleteEventsAfterEpoch 是删除路径")
	require.Equal(t, int64(2), byTable[TableFNSTokens].Rows)
	require.Equal(t, int64(1), byTable[TableFNSTokens].Deletes)
	require.Equal(t, int64(1), byTable[TableFNSActions].Rows)
	require.Equal(t, int64(1), byTable[TableFNSTransfers].Rows)
	require.Equal(t, int64(2), byTable[TableFNSReverses].Rows, "两个 Add*Reserve* 都是写")
	require.Equal(t, int64(2), byTable[TableFNSReverses].Deletes)

	require.Panics(t, func() { NewNoWriteFnsSaver(nil) })
}

func TestNoWriteFEvmRepoBlocksAllWrites(t *testing.T) {
	inner := &fakeFEvmRepo{}
	wrapped := NewNoWriteFEvmRepo(inner)

	ctx := context.Background()
	require.NoError(t, wrapped.CreateERC20TransferBatch(ctx, []*po.FEvmERC20Transfer{{}}))
	require.NoError(t, wrapped.CreateErc721TransferBatch(ctx, []*po.NFTTransfer{{}, {}}))
	require.NoError(t, wrapped.CreateErc721Tokens(ctx, []*po.NFTToken{{}, {}, {}}))
	require.NoError(t, wrapped.SaveAPISignatures(ctx, []*po.FEvmABISignature{{}, {}, {}, {}}))

	require.Zero(t, inner.writes)

	byTable := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		byTable[s.Table] = s
	}
	require.Equal(t, int64(1), byTable[TableERC20Transfers].Rows)
	require.Equal(t, int64(2), byTable[TableNFTTransfers].Rows)
	require.Equal(t, int64(3), byTable[TableNFTTokens].Rows)
	require.Equal(t, int64(4), byTable[TableABISignatures].Rows)

	require.Panics(t, func() { NewNoWriteFEvmRepo(nil) })
}

// 派生表名常量必须与 po 的 TableName 一致
func TestTableNameConstants(t *testing.T) {
	require.Equal(t, po.FNSEvent{}.TableName(), TableFNSEvents)
	require.Equal(t, po.FNSToken{}.TableName(), TableFNSTokens)
	require.Equal(t, po.FNSAction{}.TableName(), TableFNSActions)
	require.Equal(t, po.FNSTransfer{}.TableName(), TableFNSTransfers)
	require.Equal(t, po.FnsReserve{}.TableName(), TableFNSReverses)
	require.Equal(t, po.NFTTransfer{}.TableName(), TableNFTTransfers)
	require.Equal(t, po.NFTToken{}.TableName(), TableNFTTokens)
}

// buildTarget：同步器名与生产一致（fns），任务 fns-task；默认带计算器，--skip-calculator 时只有任务。
func TestBuildTargetWiring(t *testing.T) {
	db := offlinereplay.OpenTestDB(t, &offlinereplay.Recorder{})
	decoder := &fakeABIDecoder{}

	t.Run("默认含计算器", func(t *testing.T) {
		target, writes := buildTarget(db, decoder, false, false)
		require.Equal(t, "fns", target.Name)
		require.Equal(t, []string{"fns-task", "calc-fns-task"}, target.TaskNames())
		require.Nil(t, writes)
	})

	t.Run("no-write 注入两个计数包装", func(t *testing.T) {
		target, writes := buildTarget(db, decoder, true, false)
		require.Equal(t, "fns", target.Name)
		require.Equal(t, []string{"fns-task", "calc-fns-task"}, target.TaskNames())
		require.NotNil(t, writes)

		stats := writes()
		require.Len(t, stats, 9, "FEvmRepo 4 张 + FnsSaver 5 张派生表")
		for _, s := range stats {
			require.Zero(t, s.Calls, "%s 不该有写入", s.Table)
		}
	})

	t.Run("skip-calculator 只跑任务", func(t *testing.T) {
		target, writes := buildTarget(db, nil, true, true)
		require.Equal(t, []string{"fns-task"}, target.TaskNames())
		require.Empty(t, target.Calculators)
		require.NotNil(t, writes)
	})
}
