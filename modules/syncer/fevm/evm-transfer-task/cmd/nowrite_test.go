package evmtransfercmd

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	evm_transfer_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func sampleTransfers(t *testing.T) []*po.EvmTransfer {
	t.Helper()
	return []*po.EvmTransfer{
		{Epoch: 6357058, MessageCid: "bafy-msg-1", ActorID: "f0100", ActorAddress: "f410fabcde",
			UserAddress: "f1useraddress", MethodName: "InvokeContract"},
		{Epoch: 6357058, MessageCid: "bafy-msg-2", ActorID: "f0100", ActorAddress: "f410fabcde",
			UserAddress: "f1useraddress", MethodName: "InvokeContract"},
	}
}

// --no-write 的核心承诺：派生表仓储的写入次数恒为 0。
// 这条用「假 repo（真写入目标）+ 真 task」证明：写路径全被拦在包装层，底层一次都没被调用。
func TestNoWriteRepoSuppressesDerivedTableWrites(t *testing.T) {
	inner := &fakeRepo{}
	noWrite := NewNoWriteEvmTransferRepo(inner)
	task := evm_transfer_task.NewEVMTransferTask(noWrite)

	require.NoError(t, task.SaveEvmTransfers(context.Background(), sampleTransfers(t)))
	require.NoError(t, task.SaveEvmTransferStats(context.Background(), []*po.EvmTransferStat{
		{Epoch: 6357120, ActorID: "f0100", Interval: "1h"},
	}))
	require.NoError(t, task.RollBack(context.Background(), chain.Epoch(6357058)))

	// 底层仓储（真写目标）写入次数 = 0
	calls, rows := inner.Writes()
	require.Zero(t, calls, "假 repo 的写入次数必须为 0")
	require.Zero(t, rows)
	transferCalls, transferRows := inner.TransferWrites()
	require.Zero(t, transferCalls)
	require.Zero(t, transferRows)
	statCalls, statRows := inner.StatWrites()
	require.Zero(t, statCalls)
	require.Zero(t, statRows)

	// 但调用与行数被如实统计（等于真写模式会落的行）
	got := noWrite.Stats()
	require.Equal(t, int64(1), got.TransferCalls)
	require.Equal(t, int64(2), got.TransferRows)
	require.Equal(t, int64(1), got.StatCalls)
	require.Equal(t, int64(1), got.StatRows)
	require.Equal(t, int64(2), got.DeleteCalls) // RollBack 里的两次 Delete
	require.Equal(t, int64(3), got.TotalRows())
}

// 对照组：同一份 dal 在 --no-write 关闭时会真的下发 INSERT；
// 包上 NoWriteEvmTransferRepo 后一条 SQL 都发不出去。
// 这证明「SQL 记录为空」这条判据是有牙齿的（否则它只是没测到写入而已）。
func TestNoWriteRepoBlocksRealDalSQL(t *testing.T) {
	conn := &fakeConn{}
	db := newTestGormDB(t, conn)
	inner := dal.NewEVMTransferDal(db)
	transfers := sampleTransfers(t)

	// 对照组（真写路径）：dal 直接把 INSERT 下发到连接
	require.Error(t, inner.SaveEvmTransfers(context.Background(), transfers),
		"假连接对任何 SQL 都返回错误，说明确实下发过语句")
	sqls := conn.Statements()
	require.NotEmpty(t, sqls)
	require.Contains(t, strings.Join(sqls, "\n"), "INSERT")
	require.Contains(t, strings.Join(sqls, "\n"), "evm_transfers")

	// 被测路径（--no-write）：写入被拦下，连接上一片空白
	conn.Reset()
	noWrite := NewNoWriteEvmTransferRepo(inner)
	require.NoError(t, noWrite.SaveEvmTransfers(context.Background(), transfers))
	require.NoError(t, noWrite.SaveEvmTransferStats(context.Background(), []*po.EvmTransferStat{
		{Epoch: 6357120, ActorID: "f0100", Interval: "1h"},
	}))
	require.Empty(t, conn.Statements(), "--no-write 下不得向数据库下发任何语句")
	require.Equal(t, int64(2), noWrite.Stats().TransferRows)
	require.Equal(t, int64(1), noWrite.Stats().StatRows)
}

// 读操作必须原样透传（离线回放要读 fevm.evm_transfers 聚合统计，读是允许的）
func TestNoWriteRepoDelegatesReads(t *testing.T) {
	inner := &fakeRepo{}
	noWrite := NewNoWriteEvmTransferRepo(inner)
	require.Equal(t, inner, noWrite.Inner())

	_, err := noWrite.GetEvmTransferStats(context.Background(), chain.Epoch(6357058))
	require.NoError(t, err)
	require.Equal(t, 1, inner.StatsReads(), "GetEvmTransferStats 必须透传给底层仓储")
}

func TestNoWriteRepoPanicsOnNilInner(t *testing.T) {
	require.Panics(t, func() { NewNoWriteEvmTransferRepo(nil) })
}

func TestCountingAggCountsCalls(t *testing.T) {
	agg := newFakeAgg(100000, nil)
	counted := NewCountingAgg(agg)
	require.Equal(t, agg, counted.Inner())

	ctx := context.Background()
	_, err := counted.LatestTipset(ctx)
	require.NoError(t, err)
	_, err = counted.ParentTipset(ctx, chain.Epoch(6))
	require.NoError(t, err)
	_, err = counted.Tipset(ctx, chain.Epoch(6))
	require.NoError(t, err)
	_, err = counted.Traces(ctx, chain.Epoch(6), chain.Epoch(7))
	require.NoError(t, err)

	got := counted.Stats()
	require.Equal(t, AggStats{Traces: 1, Tipsets: 1, ParentTipsets: 1, LatestTipsets: 1}, got)
	require.Equal(t, int64(4), got.Total())
	require.Equal(t, []int64{6}, agg.EpochsRequested())
}

func TestCountingAggPanicsOnNilInner(t *testing.T) {
	require.Panics(t, func() { NewCountingAgg(nil) })
}
