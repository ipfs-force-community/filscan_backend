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

	// --config / --start / --end 必填；--no-write 可选且默认关闭（默认真写）
	required := []string{"config", "start", "end"}
	for _, name := range required {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, "缺少参数 %s", name)
		require.Equal(t, []string{"true"}, flag.Annotations[cobra.BashCompOneRequiredFlag],
			"参数 %s 必须标记为必填", name)
	}

	noWrite := cmd.Flags().Lookup("no-write")
	require.NotNil(t, noWrite)
	require.Equal(t, "false", noWrite.DefValue, "--no-write 默认必须是关闭（默认真写）")
	require.Empty(t, noWrite.Annotations[cobra.BashCompOneRequiredFlag], "--no-write 必须可选")

	require.Equal(t, "c", cmd.Flags().Lookup("config").Shorthand)
	require.Equal(t, "s", cmd.Flags().Lookup("start").Shorthand)
	require.Equal(t, "e", cmd.Flags().Lookup("end").Shorthand)

	// 帮助文本要点名派生表与「不写指针/台账」，避免运维误用
	for _, want := range []string{"fevm.evm_transfers", "fevm.evm_transfer_stats",
		"chain.sync_syncers", "chain.sync_task_epochs", "chain.sync_skipped_epochs", "--no-write"} {
		require.True(t, strings.Contains(cmd.Long, want), "Long 帮助应说明 %s", want)
	}

	// 缺必填参数时必须在连任何依赖之前就失败
	cmd.SetArgs(nil)
	require.Error(t, cmd.Execute())
}
