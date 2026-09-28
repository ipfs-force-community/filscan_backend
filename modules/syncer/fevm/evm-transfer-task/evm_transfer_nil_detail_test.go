package evm_transfer_task

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// TestIsEvmTransferTraceDoesNotPanicOnNilDetail 覆盖「nil 判断前访问 trace.Detail.Actor」这条 panic：
// 聚合器 trace 文档缺 Detail（或 traces 里夹空元素）时，旧实现在唯一那行就 nil 解引用，
// 由 execTaskOrCalculator 的 recover 变成整高度失败并原地重试。
func TestIsEvmTransferTraceDoesNotPanicOnNilDetail(t *testing.T) {

	// 旧表达式：strings.Split(trace.Detail.Actor, "/") —— 下面两个输入都会 panic
	nilDetail := &londobell.TraceMessage{ID: "bafyNoDetail-1", IsBlock: true}

	require.NotPanics(t, func() {
		require.False(t, isEvmTransferTrace(nilDetail), "Detail 缺失的 trace 不处理，且不得 panic")
	})

	require.NotPanics(t, func() {
		require.False(t, isEvmTransferTrace(nil), "nil trace 不处理，且不得 panic")
	})

	// 直接钉住回归形态：按旧写法这里会 panic，新写法返回 false
	require.NotPanics(t, func() {
		require.False(t, isEvmTransferTrace(&londobell.TraceMessage{
			IsBlock: true,
			Detail:  nil,
		}))
	})
}

// TestIsEvmTransferTraceKeepsOriginalSemantics 钉住判定语义与改动前一致
// （原条件：Detail.Actor 末段 == evm && Detail.Method == "InvokeContract" && IsBlock）
func TestIsEvmTransferTraceKeepsOriginalSemantics(t *testing.T) {

	cases := []struct {
		name  string
		trace *londobell.TraceMessage
		want  bool
	}{
		{
			name:  "evm 合约调用且为区块消息",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "f0100/evm", Method: "InvokeContract"}},
			want:  true,
		},
		{
			name:  "actor 路径末段为 evm（多级路径）",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "f0100/f0200/evm", Method: "InvokeContract"}},
			want:  true,
		},
		{
			name:  "非区块消息",
			trace: &londobell.TraceMessage{IsBlock: false, Detail: &londobell.MessageDetail{Actor: "f0100/evm", Method: "InvokeContract"}},
			want:  false,
		},
		{
			name:  "方法不是 InvokeContract",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "f0100/evm", Method: "Send"}},
			want:  false,
		},
		{
			name:  "调用方不是 evm",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "f0100/multisig", Method: "InvokeContract"}},
			want:  false,
		},
		{
			name:  "eam（EVM actor 创建）不算转账",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "f0100/eam", Method: "InvokeContract"}},
			want:  false,
		},
		{
			name:  "actor 为空串",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "", Method: "InvokeContract"}},
			want:  false,
		},
		{
			name:  "evm 前缀但不是末段（f0100/evm0）",
			trace: &londobell.TraceMessage{IsBlock: true, Detail: &londobell.MessageDetail{Actor: "f0100/evm0", Method: "InvokeContract"}},
			want:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, isEvmTransferTrace(c.trace))
		})
	}
}
