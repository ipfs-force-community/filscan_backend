package evmtransfercmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// 子命令的注册契约：--config/--start/--end 必填（与样板一致），--no-write 可选且默认关闭
// （默认真写：必须显式加 --no-write 才会拦下派生表写入）。
func TestCommandWiresFlagsAndRequirements(t *testing.T) {
	cmd := Command()
	require.Equal(t, "evm-transfer", cmd.Name())

	for _, name := range []string{"config", "start", "end"} {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, "缺少 --%s", name)
		require.NotNil(t, f.Annotations[cobra.BashCompOneRequiredFlag], "--%s 必须为必填", name)
	}

	require.Equal(t, "c", cmd.Flags().Lookup("config").Shorthand)
	require.Equal(t, "s", cmd.Flags().Lookup("start").Shorthand)
	require.Equal(t, "e", cmd.Flags().Lookup("end").Shorthand)

	noWrite := cmd.Flags().Lookup("no-write")
	require.NotNil(t, noWrite)
	require.Equal(t, "false", noWrite.DefValue, "--no-write 默认关闭")
	require.Nil(t, noWrite.Annotations[cobra.BashCompOneRequiredFlag], "--no-write 不应是必填项")

	// 帮助信息里要说清两种模式（运维一眼能看出哪个是安全的测量模式）
	require.Contains(t, cmd.Use, "--no-write")
	require.Contains(t, cmd.Long, "--no-write")
	require.Contains(t, cmd.Short, "不写同步指针/台账")

	// 未指定子命令参数时不应执行（flag 解析失败即返回错误，不会去连生产依赖）
	cmd.SetArgs([]string{"--start", "1"})
	require.Error(t, cmd.Execute(), "必填项缺失时必须报错")
}
