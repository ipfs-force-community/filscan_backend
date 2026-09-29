package evmtransfercmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCommandRegistrationAndFlags(t *testing.T) {
	cmd := Command()
	require.Equal(t, "evm-transfer", cmd.Name())

	// --config 必填；--start/--end（区间）与 --epochs-file（清单）二选一 ⇒ 三者都不单独标记必填，
	// 互斥与「必须给出其一」由 offline-replay.ResolvePlan 判定（见该包单测）。
	cfg := cmd.Flags().Lookup("config")
	require.NotNil(t, cfg, "缺少参数 config")
	require.Equal(t, []string{"true"}, cfg.Annotations[cobra.BashCompOneRequiredFlag],
		"参数 config 必须标记为必填")
	for _, name := range []string{"start", "end", "epochs-file"} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "缺少参数 %s", name)
		require.Empty(t, flag.Annotations[cobra.BashCompOneRequiredFlag],
			"参数 %s 不应是必填：区间与清单模式二选一", name)
	}

	noWrite := cmd.Flags().Lookup("no-write")
	require.NotNil(t, noWrite)
	require.Equal(t, "false", noWrite.DefValue, "--no-write 默认必须是关闭（默认真写）")
	require.Empty(t, noWrite.Annotations[cobra.BashCompOneRequiredFlag], "--no-write 必须可选")

	// --skip-acc-stats：必须存在、必须可选、默认必须是 false ——
	// 默认 false 是本改动的硬约束：不带该参数时行为必须与不带开关的历史版本完全一致
	// （实时同步器与既有回放用法都不受影响）。
	skipAccStats := cmd.Flags().Lookup("skip-acc-stats")
	require.NotNil(t, skipAccStats, "缺少参数 skip-acc-stats")
	require.Equal(t, "false", skipAccStats.DefValue,
		"--skip-acc-stats 默认必须关闭（否则会静默改掉每 120 高度的累计快照重算行为）")
	require.Empty(t, skipAccStats.Annotations[cobra.BashCompOneRequiredFlag], "--skip-acc-stats 必须可选")
	require.Contains(t, skipAccStats.Usage, "累计快照", "--skip-acc-stats 的说明应点明它跳过的是累计快照重算")

	require.Equal(t, "c", cmd.Flags().Lookup("config").Shorthand)
	require.Equal(t, "s", cmd.Flags().Lookup("start").Shorthand)
	require.Equal(t, "e", cmd.Flags().Lookup("end").Shorthand)

	// 帮助文本要点名派生表与「不写指针/台账」，避免运维误用；--skip-acc-stats 也要写进帮助
	for _, want := range []string{"fevm.evm_transfers", "fevm.evm_transfer_stats",
		"chain.sync_syncers", "chain.sync_task_epochs", "chain.sync_skipped_epochs",
		"--no-write", "--epochs-file", "--skip-acc-stats"} {
		require.True(t, strings.Contains(cmd.Long, want), "Long 帮助应说明 %s", want)
	}

	// Use 行要带上下开关，便于 --help 一眼看到
	require.Contains(t, cmd.Use, "--skip-acc-stats")

	// 缺必填参数时必须在连任何依赖之前就失败
	cmd.SetArgs(nil)
	require.Error(t, cmd.Execute())
}
