package erc20

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// fakeAgg 只覆写本用例用到的方法：内嵌接口让其余方法保持「未实现即 panic」，
// 一旦被测代码多调了一个接口方法，测试会立刻暴露而不是静默通过。
type fakeAgg struct {
	londobell.Agg
	receipt *londobell.EthReceipt
	err     error
	calls   int
}

func (f *fakeAgg) GetTransactionReceiptByCid(ctx context.Context, cid string) (*londobell.EthReceipt, error) {
	f.calls++
	return f.receipt, f.err
}

func epoch() chain.Epoch {
	return chain.Epoch(6259665)
}

// TestSelectHandleTracesSkipsUnsuccessfulEVM 覆盖「控制器不看 ExitCode」这条主网卡死根因：
// 失败交易的 trace 必须在取回执之前被剔除，成功交易照旧保留。
func TestSelectHandleTracesSkipsUnsuccessfulEVM(t *testing.T) {

	// 以下 trace 组成与聚合器实际返回的形状一致：
	// 同一消息的 depth-1 trace 与它的子调用共享 ID 的第二段（"cid-<idx>" 与 "cid-<idx>-<seq>"）。
	// 失败消息（ExitCode=21，实测 6259665 那条）：
	evmSubOfFailedMsg := &londobell.TraceMessage{
		ID: "bafyMsgA-3-0-0", Depth: 2, Actor: "f0100/evm",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	failedMsg := &londobell.TraceMessage{
		ID: "bafyMsgA-3", Depth: 1, Cid: "bafyMsgA",
		MsgRct: &londobell.MsgRct{ExitCode: 21},
	}
	// 成功消息（应保留）：
	evmSubOfOkMsg := &londobell.TraceMessage{
		ID: "bafyMsgB-4-0-0", Depth: 2, Actor: "f0100/evm",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	okMsg := &londobell.TraceMessage{
		ID: "bafyMsgB-4", Depth: 1, Cid: "bafyMsgB", SignedCid: strPtr("bafyMsgBSigned"),
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	// 无回执的消息（无法判定成功，同样不应去取回执）：
	noRctMsg := &londobell.TraceMessage{
		ID: "bafyMsgC-5", Depth: 1, Cid: "bafyMsgC", Actor: "f0100/evm",
	}
	// 非 evm 参与、且不在 evm 调用树里的消息（本就不选）：
	unrelatedMsg := &londobell.TraceMessage{
		ID: "bafyMsgD-6", Depth: 1, Cid: "bafyMsgD", Actor: "f099/miner",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	// 深度不是 1 的 trace（不单独处理）：
	deepTrace := &londobell.TraceMessage{
		ID: "bafyMsgA-3-1", Depth: 3, Actor: "f0100/evm",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	// ID 不含 "-" 的异常 trace（应只打日志、不 panic、不入选）：
	brokenIdTrace := &londobell.TraceMessage{
		ID: "brokenid", Depth: 1, Actor: "f0100/evm",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}

	traces := []*londobell.TraceMessage{
		nil, // 空指针不得 panic
		evmSubOfFailedMsg,
		failedMsg,
		evmSubOfOkMsg,
		okMsg,
		noRctMsg,
		unrelatedMsg,
		deepTrace,
		brokenIdTrace,
	}

	task := &ERC20Task{}
	ctx := syncer.NewTestContext(nil, nil, epoch())

	targets := task.selectHandleTraces(ctx, traces)

	require.Len(t, targets, 1, "只有执行成功的 depth-1 EVM 消息才该被处理")
	require.Same(t, okMsg, targets[0])
	// 失败交易不得入选 —— 这就是「不看 ExitCode」的修复点：
	for _, v := range targets {
		require.NotEqual(t, 21, v.MsgRct.ExitCode)
	}
}

// TestSelectHandleTracesKeepsEvmTreeSelection 钉住原选择语义未被改动：
// 消息必须在「调用树里出现过 evm/eam actor」时才处理。
func TestSelectHandleTracesKeepsEvmTreeSelection(t *testing.T) {

	task := &ERC20Task{}
	ctx := syncer.NewTestContext(nil, nil, epoch())

	// 调用树里没有 evm/eam actor 的消息：即便 depth-1、执行成功也不处理
	noEvmTree := &londobell.TraceMessage{
		ID: "bafyX-7", Depth: 1, Cid: "bafyX", Actor: "f0200/multisig",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	require.Empty(t, task.selectHandleTraces(ctx, []*londobell.TraceMessage{noEvmTree}))

	// 调用树里有 eam actor（创建 EVM actor 的消息）时同样处理
	eamSub := &londobell.TraceMessage{ID: "bafyY-8-0", Depth: 2, Actor: "f0300/eam"}
	root := &londobell.TraceMessage{
		ID: "bafyY-8", Depth: 1, Cid: "bafyY",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}
	targets := task.selectHandleTraces(ctx, []*londobell.TraceMessage{eamSub, root})
	require.Len(t, targets, 1)
	require.Same(t, root, targets[0])
}

// TestHandleERC20TransferReceiptErrorDoesNotBlockEpoch 覆盖卡死机制本身：
// 单条 trace 的回执取不到（聚合器对失败交易回 code:1 / 回执解码失败）只应留 Warn 并跳过，
// 不得把整个高度判失败 —— 旧实现在这里上抛错误，正是 erc20 卡 52 天的直接机制。
func TestHandleERC20TransferReceiptErrorDoesNotBlockEpoch(t *testing.T) {

	// 聚合器对失败交易回执的真实报错形状（HTTP 200 但业务码非 0）
	decodeFailure := &londobell.BusinessError{
		Code:    "1",
		Message: "expected byte array",
		URL:     "http://agg/aggregators/receipt",
	}
	agg := &fakeAgg{err: decodeFailure}

	task := &ERC20Task{}
	ctx := syncer.NewTestContext(nil, agg, epoch())
	trace := &londobell.TraceMessage{
		ID: "bafyMsgA-3", Cid: "bafyMsgA",
		MsgRct: &londobell.MsgRct{ExitCode: 21},
	}

	balances, transfers, swaps, err := task.handleERC20Transfer(ctx, trace)

	require.NoError(t, err, "回执解码失败必须降级为可诊断的跳过，而不是让整高度失败")
	require.Nil(t, balances)
	require.Nil(t, transfers)
	require.Nil(t, swaps)
	require.Equal(t, 1, agg.calls)
}

// TestHandleERC20TransferKeepsTransportErrorRetryable 保证降级不过界：
// 传输级错误（聚合器不通/超时）仍要上抛以保持重试，不能当作数据问题静默吞掉。
func TestHandleERC20TransferKeepsTransportErrorRetryable(t *testing.T) {

	agg := &fakeAgg{err: fmt.Errorf("post receipt: %w", context.DeadlineExceeded)}

	task := &ERC20Task{}
	ctx := syncer.NewTestContext(nil, agg, epoch())
	trace := &londobell.TraceMessage{
		ID: "bafyMsgB-4", Cid: "bafyMsgB",
		MsgRct: &londobell.MsgRct{ExitCode: 0},
	}

	_, _, _, err := task.handleERC20Transfer(ctx, trace)

	require.Error(t, err)
	require.Equal(t, syncer.ErrorKindTransport, syncer.ClassifySyncError(err),
		"传输级错误必须保持可重试语义（不计入跳过防线）")
}

func strPtr(v string) *string { return &v }
