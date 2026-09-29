package evm_transfer_task

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 本文件钉住 --skip-acc-stats 的判定语义（两个方向都要钉死）。
//
// 为什么不在这里「真跑一遍 Exec」：Exec 需要 *syncer.Context，而 syncer.Context 的
// datamap 字段没有导出构造入口（Datamap 只在 syncer.go 内用 &Datamap{} 建、经未导出的
// prepareTaskContext 注入），从本包既造不出带 datamap 的 Context，也造不出 ctx.Adapter()。
// 所以这里对 Exec 使用的唯一判定入口 shouldCalcAccStats 做单测，并由 cmd 包
// （skipacc_replay_test.go）用真实 EVMTransferTask 跑完整 Dry 管线，在仓储层钉住
// 「边界高度到底有没有读派生表重算累计快照」，两层合起来覆盖 Exec 的可观察行为。

// 6357000 = 120 × 52975，是线上真实区间的「120 边界」高度之一，用来钉住判定在
// 生产量级的高度上同样成立（不受浮点/溢出之类问题影响）。
const skipAccBoundaryEpoch = int64(6357000)

// taskWithSkip 构造带开关的任务（包内可直接写未导出字段，无需为测试新增生产 API）
func taskWithSkip(skip bool) EVMTransferTask {
	return EVMTransferTask{skipAccStats: skip}
}

// 开关打开 + 高度是 120 的整数倍 ⇒ 不得走累计快照路径。
func TestShouldCalcAccStatsSkipsOnBoundaryEpochWhenEnabled(t *testing.T) {
	task := taskWithSkip(true)

	require.False(t, task.shouldCalcAccStats(120), "开关打开时 120 的整数倍高度必须跳过累计快照")
	require.False(t, task.shouldCalcAccStats(240))
	require.False(t, task.shouldCalcAccStats(skipAccBoundaryEpoch))
	require.False(t, task.shouldCalcAccStats(0), "0 也是 120 的整数倍（原表达式 0%120==0 为真）")
}

// 开关关闭（默认）+ 高度是 120 的整数倍 ⇒ 仍然调用累计快照路径（默认行为不变）。
func TestShouldCalcAccStatsKeepsRecalculatingOnBoundaryEpochWhenDisabled(t *testing.T) {
	task := taskWithSkip(false)

	require.True(t, task.shouldCalcAccStats(120), "开关关闭时 120 的整数倍高度必须照旧重算累计快照")
	require.True(t, task.shouldCalcAccStats(240))
	require.True(t, task.shouldCalcAccStats(skipAccBoundaryEpoch))
	require.True(t, task.shouldCalcAccStats(0), "0 也满足 %120==0，语义与改动前逐字一致")
}

// 非 120 整数倍高度：开关开或关都不触发累计快照（保持改动前的判定，开关不影响非边界高度）。
func TestShouldCalcAccStatsNeverFiresOnNonBoundaryEpoch(t *testing.T) {
	for _, skip := range []bool{false, true} {
		task := taskWithSkip(skip)

		for _, epoch := range []int64{1, 119, 121, 239, 241, 6356999, 6357001, 6357058, 6357062} {
			require.Falsef(t, task.shouldCalcAccStats(epoch),
				"非 120 整数倍高度 %d 不该重算累计快照（skip=%v）", epoch, skip)
		}
	}
}

// 公开构造路径（NewEVMTransferTask + WithSkipAccStats 链式）必须与包内构造语义一致，
// 且默认值必须是 false —— 这是「默认行为完全不变」的根证据。
func TestSkipAccStatsDefaultsToFalseViaPublicConstructor(t *testing.T) {
	// repo 传 nil 是安全的：本用例只碰开关与判定，不碰仓储
	dflt := NewEVMTransferTask(nil)
	require.False(t, dflt.skipAccStats, "零值必须是「不跳过」——默认行为不得改变")
	require.True(t, dflt.shouldCalcAccStats(120), "未经 WithSkipAccStats 的任务，120 边界高度仍要重算累计快照")

	skipped := NewEVMTransferTask(nil).WithSkipAccStats(true)
	require.False(t, skipped.shouldCalcAccStats(120), "链式打开开关后 120 边界高度必须跳过")
	require.False(t, skipped.shouldCalcAccStats(121), "非边界高度判定不受开关影响（121%120=1，本就不重算）")

	require.True(t, dflt.shouldCalcAccStats(120), "对另一实例调开关不得改变原有实例")
}

// WithSkipAccStats 必须可链式调用（返回同一指针），且可来回翻转。
func TestWithSkipAccStatsChainsAndToggles(t *testing.T) {
	task := &EVMTransferTask{}
	require.Same(t, task, task.WithSkipAccStats(true).WithSkipAccStats(false).WithSkipAccStats(true))

	require.True(t, task.skipAccStats)
	require.False(t, task.shouldCalcAccStats(120))

	task.WithSkipAccStats(false)
	require.True(t, task.shouldCalcAccStats(120), "关回去后必须立刻恢复原判定")

	// 两个实例互不影响（开关只落在自己身上）
	other := &EVMTransferTask{}
	require.False(t, other.skipAccStats, "开关不得外溢到其它任务实例")
}

// 值接收者语义提醒：shouldCalcAccStats 是值接收者，读的是调用时刻的字段快照。
// 只有在任务被存进 syncer.Task 之前设置开关才生效 —— buildTarget 的链式调用正是这个顺序。
func TestShouldCalcAccStatsReadsCurrentFlagValue(t *testing.T) {
	task := &EVMTransferTask{}
	require.True(t, (*task).shouldCalcAccStats(120))

	task.WithSkipAccStats(true)
	require.False(t, (*task).shouldCalcAccStats(120))

	// 副本拿到的是设置后的值（值复制照常携带开关）
	copied := *task
	require.False(t, copied.shouldCalcAccStats(120))
}
