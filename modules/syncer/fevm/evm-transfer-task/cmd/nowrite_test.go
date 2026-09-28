package evmtransfercmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// fakeEvmTransferRepo 假仓储：只记录「底层到底被写了几次 / 读了几次」。
// 它代表 fevm.evm_transfers / fevm.evm_transfer_stats 的落库通道 ——
// --no-write 模式的核心契约就是这些写计数恒为 0。
type fakeEvmTransferRepo struct {
	// 嵌入接口：本用例只关心写方法，其余（如 CountTxsOfContracts）不实现，被调用即 panic
	repository.EvmTransferRepo

	writes int
	reads  int
	calls  []string
}

func (f *fakeEvmTransferRepo) SaveEvmTransfers(_ context.Context, _ []*po.EvmTransfer) error {
	f.writes++
	f.calls = append(f.calls, "SaveEvmTransfers")
	return nil
}

func (f *fakeEvmTransferRepo) SaveEvmTransferStats(_ context.Context, _ []*po.EvmTransferStat) error {
	f.writes++
	f.calls = append(f.calls, "SaveEvmTransferStats")
	return nil
}

func (f *fakeEvmTransferRepo) DeleteEvmTransfers(_ context.Context, _ chain.Epoch) error {
	f.writes++
	f.calls = append(f.calls, "DeleteEvmTransfers")
	return nil
}

func (f *fakeEvmTransferRepo) DeleteEvmTransferStats(_ context.Context, _ chain.Epoch) error {
	f.writes++
	f.calls = append(f.calls, "DeleteEvmTransferStats")
	return nil
}

func (f *fakeEvmTransferRepo) GetEvmTransferStats(_ context.Context, _ chain.Epoch) ([]*bo.EVMTransferStats, error) {
	f.reads++
	f.calls = append(f.calls, "GetEvmTransferStats")
	return nil, nil
}

func TestNoWriteEvmTransferRepoBlocksAllWrites(t *testing.T) {
	inner := &fakeEvmTransferRepo{}
	wrapped := NewNoWriteEvmTransferRepo(inner)

	ctx := context.Background()
	require.NoError(t, wrapped.SaveEvmTransfers(ctx, []*po.EvmTransfer{{}, {}, {}}))
	require.NoError(t, wrapped.SaveEvmTransferStats(ctx, []*po.EvmTransferStat{{}, {}}))
	require.NoError(t, wrapped.DeleteEvmTransfers(ctx, chain.Epoch(100)))
	require.NoError(t, wrapped.DeleteEvmTransferStats(ctx, chain.Epoch(100)))

	require.Zero(t, inner.writes, "底层仓储一次写都不该被调用")
	require.Empty(t, inner.calls)

	// 计数如实记录「本来会写多少行」
	calls, rows, deletes := wrapped.TransferStats()
	require.Equal(t, int64(1), calls)
	require.Equal(t, int64(3), rows)
	require.Equal(t, int64(1), deletes)

	calls, rows, _ = wrapped.StatStats()
	require.Equal(t, int64(1), calls)
	require.Equal(t, int64(2), rows)

	stats := wrapped.WriteStats()
	require.Len(t, stats, 2)
	require.Equal(t, TableEvmTransfers, stats[0].Table)
	require.Equal(t, int64(3), stats[0].Rows)
	require.Equal(t, TableEvmTransferStats, stats[1].Table)
	require.Equal(t, int64(2), stats[1].Rows)
}

func TestNoWriteEvmTransferRepoPassesReadsThrough(t *testing.T) {
	inner := &fakeEvmTransferRepo{}
	wrapped := NewNoWriteEvmTransferRepo(inner)

	got, err := wrapped.GetEvmTransferStats(context.Background(), chain.Epoch(6357058))
	require.NoError(t, err)
	require.Nil(t, got)
	require.Equal(t, 1, inner.reads, "读操作必须透传到底层（离线回放要读派生表算累计快照）")
	require.Equal(t, inner, wrapped.Inner())

	require.Panics(t, func() { NewNoWriteEvmTransferRepo(nil) })
}

// 派生表名常量必须与 po 的 TableName 一致（避免报告里印错表名误导运维）
func TestTableNameConstants(t *testing.T) {
	require.Equal(t, po.EvmTransfer{}.TableName(), TableEvmTransfers)
	require.Equal(t, po.EvmTransferStat{}.TableName(), TableEvmTransferStats)
}
