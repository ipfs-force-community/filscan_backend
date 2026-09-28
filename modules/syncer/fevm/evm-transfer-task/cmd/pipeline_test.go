package evmtransfercmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	evmtransfertask "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件用真实的 EVMTransferTask 跑完整条 Dry 管线，是「离线回放真能补数据、且只写派生表」的主证据。
//
// 高度刻意不取 120 的整数倍：Task 在 epoch%120==0 时会读派生表重算 1 小时累计快照（真实 DB 路径），
// 那一段由生产/真库验证，不放进离线单测。
const (
	replayFrom = 6357058
	replayTo   = 6357062 // 5 个高度
)

func evmTrace(epoch int64, i int) *londobell.TraceMessage {
	return &londobell.TraceMessage{
		Cid:     fmt.Sprintf("bafy-replay-%d-%d", epoch, i),
		Epoch:   epoch,
		From:    chain.SmartAddress(fmt.Sprintf("t0100%d", i)),
		To:      chain.SmartAddress("t0999"),
		Value:   decimal.NewFromInt(int64(100 + i)),
		IsBlock: true,
		Detail:  &londobell.MessageDetail{Actor: "t099/evm", Method: "InvokeContract"},
		GasCost: &londobell.GasCost{TotalCost: decimal.NewFromInt(2)},
		MsgRct:  &londobell.MsgRct{ExitCode: 0},
	}
}

func fakeActorState() *londobell.ActorState {
	return &londobell.ActorState{
		ActorID:       "t0999",
		DelegatedAddr: "0x0000000000000000000000000000000000000999",
		Balance:       decimal.NewFromInt(1000),
	}
}

// buildReplay 用真实的 EVMTransferTask + 假聚合器/假适配器/假仓储组装 Dry 同步器。
// noWrite=true 时仓储被「只统计不落库」包装替换。
func buildReplay(t *testing.T, rec *offlinereplay.Recorder, adapter *offlinereplay.FakeAdapter,
	noWrite bool) (*filscansyncer.Syncer, *offlinereplay.Telemetry, *fakeEvmTransferRepo, *NoWriteEvmTransferRepo) {

	t.Helper()

	const tracesPerEpoch = 2
	agg := offlinereplay.NewFakeAgg(6409045, func(epoch chain.Epoch) []*londobell.TraceMessage {
		out := make([]*londobell.TraceMessage, 0, tracesPerEpoch)
		for i := 0; i < tracesPerEpoch; i++ {
			out = append(out, evmTrace(epoch.Int64(), i))
		}
		return out
	})

	inner := &fakeEvmTransferRepo{}
	var repo repository.EvmTransferRepo = inner
	var wrapped *NoWriteEvmTransferRepo
	if noWrite {
		wrapped = NewNoWriteEvmTransferRepo(repo)
		repo = wrapped
	}

	opt := offlinereplay.RunOptions{
		From: replayFrom, To: replayTo, NoWrite: noWrite,
		EpochsChunk: 2, EpochsThreshold: 5, ErrorWait: time.Millisecond,
	}
	target := offlinereplay.Target{
		Name:   filscansyncer.EvmContractSyncer,
		Groups: []filscansyncer.TaskGroup{{evmtransfertask.NewEVMTransferTask(repo)}},
	}

	s, tel, err := offlinereplay.Assemble(opt, target, offlinereplay.OpenTestDB(t, rec), agg, adapter)
	require.NoError(t, err)
	if wrapped != nil {
		tel.Writes = wrapped.WriteStats
	}
	require.NoError(t, s.Init())
	return s, tel, inner, wrapped
}

// --no-write：真任务跑满 5 个高度，派生数据只被计数、一行都没落库，且一条 SQL 都没下发。
func TestReplayNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rec := &offlinereplay.Recorder{}
	adapter := &offlinereplay.FakeAdapter{State: fakeActorState()}

	s, tel, inner, wrapped := buildReplay(t, rec, adapter, true)
	offlinereplay.RunSyncerAndWait(t, s.Run, 60*time.Second)

	require.Zero(t, inner.Writes(), "真写通道一次都不该被碰")
	require.Equal(t, 10, adapterCallCount(adapter))

	calls, rows, deletes := wrapped.TransferStats()
	require.Equal(t, int64(5), calls, "每个高度一次 SaveEvmTransfers")
	require.Equal(t, int64(10), rows, "每高度 2 条 trace ⇒ 2 行 fevm.evm_transfers")
	require.Zero(t, deletes)

	calls, rows, _ = wrapped.StatStats()
	require.Zero(t, calls, "区间内无 120 的整数倍高度 ⇒ 不该写累计快照")
	require.Zero(t, rows)

	require.Empty(t, rec.Statements(), "离线回放不允许向数据库下发任何语句（指针/台账/任务高度/派生表）")
	begins, commits, rollbacks := rec.TxCounts()
	require.Equal(t, 5, begins, "每高度 1 个任务 ⇒ 1 个事务")
	require.Equal(t, 5, commits)
	require.Zero(t, rollbacks)

	report := tel.Report(offlinereplay.Range{From: replayFrom, To: replayTo}, 1500*time.Millisecond)
	require.Contains(t, report, "同步器/任务     : evm-contract / evm-transfer-task")
	require.Contains(t, report, fmt.Sprintf("[%d, %d] 左闭右闭，共 5 个高度", replayFrom, replayTo))
	require.Contains(t, report, "fevm.evm_transfers 写 5 次/10 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 10 行（本次未落库）")
	require.Contains(t, report, "--no-write 只统计不落库")
}

// 不加 --no-write（真写通道 = 假仓储）时：Dry 依然保证不写同步指针 / 任务高度 / 跳过台账。
func TestDryModeSkipsPointerAndLedgerWritesEvenInWriteMode(t *testing.T) {
	rec := &offlinereplay.Recorder{}
	adapter := &offlinereplay.FakeAdapter{State: fakeActorState()}

	s, _, inner, _ := buildReplay(t, rec, adapter, false)
	offlinereplay.RunSyncerAndWait(t, s.Run, 60*time.Second)

	require.Equal(t, 5, inner.Writes(), "真写模式下派生表写入走真实仓储通道")
	require.Empty(t, rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// task 报错（节点侧历史状态不可用）时不写指针/台账，重试后仍把区间跑完。
func TestReplayRetriesOnTaskErrorWithoutWritingPointerOrLedger(t *testing.T) {
	rec := &offlinereplay.Recorder{}
	adapter := &offlinereplay.FakeAdapter{
		State:   fakeActorState(),
		ErrOnce: fmt.Errorf("failed to load state tree: failed to load hamt node bafyreplay: not found"),
	}

	s, _, inner, wrapped := buildReplay(t, rec, adapter, true)
	offlinereplay.RunSyncerAndWait(t, s.Run, 60*time.Second)

	require.Zero(t, inner.Writes())
	_, rows, _ := wrapped.TransferStats()
	// 基线 10 行（5 个高度 × 2 条 trace）。这里刻意断言 12：批次内某个高度失败后整批重跑，
	// Dry 模式**不做**「该高度任务是否已执行」的跳过判断（syncer 里该判断被 !s.dry 守着），
	// 于是同批里已成功的高度会被再写一遍 ⇒ 多 2 行。
	// 这正是 fevm.evm_transfers 上 message_cid 唯一索引会让「真写 + 重试」撞唯一键的原因，
	// 也是回放后必须核对行数、必要时按区间先删再跑的理由。
	require.Equal(t, int64(12), rows, "报错高度重试后区间跑完；批次重跑会重复写同批已成功的高度")
	require.Empty(t, rec.Statements(), "报错不得写指针/台账/任务高度")
	_, _, rollbacks := rec.TxCounts()
	require.GreaterOrEqual(t, rollbacks, 1, "失败批次应回滚事务")
}

func adapterCallCount(a *offlinereplay.FakeAdapter) int {
	calls, _ := a.Calls()
	return calls
}
