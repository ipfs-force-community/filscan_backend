package evmtransfercmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	evmtransfertask "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件是 --skip-acc-stats 在**真实管线**上的主证据：用真的 EVMTransferTask 跑完整 Dry 回放，
// 在仓储层观察「120 边界高度到底有没有读派生表重算 1 小时累计快照」。
//
// 与 pipeline_test.go 的分工：那边刻意选不含 120 整数倍的高度（避免真实 DB 路径），
// 这里**必须**选含 120 整数倍的高度，才能钉住开关只影响这一段。仓储是假实现
// （GetEvmTransferStats 直接返回 nil），所以「真实 DB 路径」不会被触发，纯离线。

// skipAccStatsBoundaryEpoch = 120 × 52975，落在线上真实回放区间里的「120 边界」高度；
// 区间 [Boundary, Boundary+4] 内只有它一个是 120 的整数倍。
const skipAccStatsBoundaryEpoch = int64(6357000)

// runBoundaryReplay 用真实 EVMTransferTask 跑一遍 [Boundary, Boundary+4]，返回假仓储。
// skipAccStats 直接走 WithSkipAccStats（与 buildTarget 里的装配顺序一致）。
func runBoundaryReplay(t *testing.T, skipAccStats bool) *fakeEvmTransferRepo {
	t.Helper()

	require.Zero(t, skipAccStatsBoundaryEpoch%120,
		"前提：起始高度必须是 120 的整数倍（也就是需要重算累计快照的边界高度）")

	rec := &offlinereplay.Recorder{}
	adapter := &offlinereplay.FakeAdapter{State: fakeActorState()}

	const tracesPerEpoch = 2
	agg := offlinereplay.NewFakeAgg(6409045, func(epoch chain.Epoch) []*londobell.TraceMessage {
		out := make([]*londobell.TraceMessage, 0, tracesPerEpoch)
		for i := 0; i < tracesPerEpoch; i++ {
			out = append(out, evmTrace(epoch.Int64(), i))
		}
		return out
	})

	inner := &fakeEvmTransferRepo{}
	target := offlinereplay.Target{
		Name: filscansyncer.EvmContractSyncer,
		Groups: []filscansyncer.TaskGroup{{
			evmtransfertask.NewEVMTransferTask(inner).WithSkipAccStats(skipAccStats),
		}},
	}
	opt := offlinereplay.RunOptions{
		From: skipAccStatsBoundaryEpoch, To: skipAccStatsBoundaryEpoch + 4,
		EpochsChunk: 2, EpochsThreshold: 5, ErrorWait: time.Millisecond,
	}

	s, _, err := offlinereplay.Assemble(opt, target, offlinereplay.OpenTestDB(t, rec), agg, adapter)
	require.NoError(t, err)
	require.NoError(t, s.Init())

	offlinereplay.RunSyncerAndWait(t, s.Run, 60*time.Second)

	// 前提校验：区间确实跑满、转账派生数据照常落库 —— 证明开关只掐掉累计快照那一段，
	// 没有把整个任务一起掐掉。
	require.Equal(t, 5, countCalls(inner, "SaveEvmTransfers"),
		"5 个高度各写一次 fevm.evm_transfers，开关不得影响主写入路径")
	require.Empty(t, rec.Statements(), "离线回放不得向数据库下发任何语句")

	return inner
}

// countCalls 假仓储调用序列里某个方法的出现次数
func countCalls(repo *fakeEvmTransferRepo, method string) int {
	n := 0
	for _, name := range repo.Calls() {
		if name == method {
			n++
		}
	}
	return n
}

// 开关关闭（默认）：边界高度照旧读派生表重算 1 小时累计快照 —— 默认行为与改动前一致。
func TestSkipAccStatsOffStillRecalculatesAccSnapshot(t *testing.T) {
	inner := runBoundaryReplay(t, false)

	require.Equal(t, 1, inner.Reads(),
		"开关关闭时，区间内唯一的 120 整数倍高度必须照旧读派生表重算累计快照（默认行为不变）")
	require.Equal(t, 1, countCalls(inner, "GetEvmTransferStats"),
		"累计快照路径必须真的走到 GetEvmTransferStats")
}

// 开关打开：边界高度一次都不读派生表 —— 累计快照路径整段被跳过。
func TestSkipAccStatsOnSkipsAccSnapshotRecalculation(t *testing.T) {
	inner := runBoundaryReplay(t, true)

	require.Zero(t, inner.Reads(),
		"开关打开后，120 整数倍高度不得再读派生表重算累计快照（--skip-acc-stats 的核心语义）")
	require.Zero(t, countCalls(inner, "GetEvmTransferStats"),
		"累计快照路径的读必须一次都不发生")
}

// 两个方向放在同一个用例里对照：唯一的差别就是开关本身，读数必须不同。
func TestSkipAccStatsOnlyChangesAccSnapshotReads(t *testing.T) {
	off := runBoundaryReplay(t, false).Reads()
	on := runBoundaryReplay(t, true).Reads()

	require.Equal(t, 1, off, "默认：边界高度重算 1 次累计快照")
	require.Equal(t, 0, on, "开关打开：0 次")
	require.NotEqual(t, off, on, "唯一差别是开关；读数必须真的有差别，否则开关没接线")
}
