package offlinereplay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---- 高度清单解析（--epochs-file）----

func TestParseEpochList(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		wantErr    string
		want       []int64
		skipped    int
		duplicates int
	}{
		{
			name: "正常清单（一行一个高度，升序）",
			text: "6280001\n6280003\n6280007\n",
			want: []int64{6280001, 6280003, 6280007},
		},
		{
			name: "乱序输入按升序排列",
			text: "6280007\n6280001\n6280003\n",
			want: []int64{6280001, 6280003, 6280007},
		},
		{
			name: "重复高度合并（只跑一次）",
			text: "10\n7\n10\n7\n10\n",
			want: []int64{7, 10}, duplicates: 3,
		},
		{
			name:    "空行与注释（整行 # / 行尾 # / 前后空白 / CRLF）",
			text:    "# 缺口高度清单（由 SQL 生成）\r\n\r\n  6280001  \r\n6280003 # 这一格是毒高度留下的\r\n	\r\n# 结尾注释\n6280007\r\n",
			want:    []int64{6280001, 6280003, 6280007},
			skipped: 4,
		},
		{
			name: "单个高度",
			text: "1\n",
			want: []int64{1},
		},
		{name: "空文件", text: "", wantErr: "没有解析出任何高度"},
		{name: "只有空行", text: "\n\n   \n", wantErr: "没有解析出任何高度"},
		{name: "只有注释", text: "# a\n# b\n", wantErr: "没有解析出任何高度"},
		{name: "非数字", text: "6280001\nabc\n", wantErr: "第 2 行不是合法高度"},
		{name: "带单位的高度（不允许）", text: "6280001h\n", wantErr: "第 1 行不是合法高度"},
		{name: "浮点高度（不允许）", text: "6280001.0\n", wantErr: "第 1 行不是合法高度"},
		{name: "负高度", text: "-1\n", wantErr: "高度必须 > 0"},
		{name: "零高度", text: "0\n", wantErr: "高度必须 > 0"},
		{name: "一条非法即整份报错（不静默跳过）", text: "10\n# c\n20\nxyz\n30\n", wantErr: "第 4 行不是合法高度"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseEpochList(c.text, "test.list")
			if c.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.wantErr)
				require.Equal(t, 0, got.Count(), "解析失败时不得返回半成品清单")
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got.Epochs())
			require.Equal(t, len(c.want), got.Count())
			require.Equal(t, c.want[0], got.Min())
			require.Equal(t, c.want[len(c.want)-1], got.Max())
			require.Equal(t, c.skipped, got.Skipped())
			require.Equal(t, c.duplicates, got.Duplicates())
			require.NotEmpty(t, got.String())
		})
	}
}

func TestParseEpochListReportsLineNumberAndSource(t *testing.T) {
	_, err := ParseEpochList("1\n2\nbad\n", "/tmp/gap.list")
	require.Error(t, err)
	require.Contains(t, err.Error(), "/tmp/gap.list", "报错要点名清单来源")
	require.Contains(t, err.Error(), "第 3 行", "报错要点名行号")
	require.Contains(t, err.Error(), "bad", "报错要带原文，便于直接定位")
}

// 清单内部必须升序去重（顺序只为可重复；同一高度跑两遍没有意义）
func TestEpochListIsSortedAndDeduped(t *testing.T) {
	got, err := ParseEpochList("9\n3\n9\n5\n3\n", "x")
	require.NoError(t, err)
	require.Equal(t, []int64{3, 5, 9}, got.Epochs())

	// Epochs() 返回副本：调用方改不动内部状态
	epochs := got.Epochs()
	epochs[0] = 999999
	require.Equal(t, []int64{3, 5, 9}, got.Epochs())
}

func TestEpochListString(t *testing.T) {
	list, err := ParseEpochList("# c\n6280001\n6280001\n6280005\n", "/tmp/gap.list")
	require.NoError(t, err)
	s := list.String()
	require.Contains(t, s, "/tmp/gap.list")
	require.Contains(t, s, "2 个高度（升序去重后）")
	require.Contains(t, s, "跨 [6280001, 6280005]")
	require.Contains(t, s, "合并重复高度 1 个")

	require.Contains(t, EpochList{source: "empty.list"}.String(), "高度清单为空")
}

func TestLoadEpochList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gap.list")
	require.NoError(t, os.WriteFile(path, []byte("# 注释\n6280003\n\n6280001\n"), 0o600))

	got, err := LoadEpochList(path)
	require.NoError(t, err)
	require.Equal(t, []int64{6280001, 6280003}, got.Epochs())
	require.Equal(t, path, got.Source())

	t.Run("文件不存在", func(t *testing.T) {
		_, err := LoadEpochList(filepath.Join(dir, "nope.list"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "读取高度清单")
	})
	t.Run("路径为空", func(t *testing.T) {
		_, err := LoadEpochList("  ")
		require.Error(t, err)
		require.Contains(t, err.Error(), "路径为空")
	})
	t.Run("空文件", func(t *testing.T) {
		empty := filepath.Join(dir, "empty.list")
		require.NoError(t, os.WriteFile(empty, []byte("\n\n"), 0o600))
		_, err := LoadEpochList(empty)
		require.Error(t, err)
		require.Contains(t, err.Error(), "没有解析出任何高度")
	})
}

// ---- 计划装配：区间模式 与 清单模式 互斥 ----

func TestResolvePlan(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "gap.list")
	require.NoError(t, os.WriteFile(listPath, []byte("6280001\n6280003\n"), 0o600))
	emptyPath := filepath.Join(dir, "empty.list")
	require.NoError(t, os.WriteFile(emptyPath, []byte("\n"), 0o600))

	t.Run("只给 --start/--end ⇒ 区间模式", func(t *testing.T) {
		plan, err := ResolvePlan(6357058, 6408647, "")
		require.NoError(t, err)
		require.False(t, plan.IsList())
		require.Equal(t, Range{From: 6357058, To: 6408647}, plan.Range)
		require.Equal(t, int64(51590), plan.Count())
		require.Equal(t, "[6357058, 6408647] 共 51590 个高度", plan.String())
	})

	t.Run("只给 --epochs-file ⇒ 清单模式", func(t *testing.T) {
		plan, err := ResolvePlan(0, 0, listPath)
		require.NoError(t, err)
		require.True(t, plan.IsList())
		require.Equal(t, []int64{6280001, 6280003}, plan.List.Epochs())
		require.Equal(t, int64(2), plan.Count())
		require.Contains(t, plan.String(), "2 个高度（升序去重后）")
	})

	t.Run("--epochs-file 与 --start/--end 同时给出 ⇒ 互斥报错（两个都拦）", func(t *testing.T) {
		_, err := ResolvePlan(6280001, 6280003, listPath)
		require.Error(t, err)
		require.Contains(t, err.Error(), "互斥")

		_, err = ResolvePlan(6280001, 0, listPath)
		require.Error(t, err)
		require.Contains(t, err.Error(), "互斥")

		_, err = ResolvePlan(0, 6280003, listPath)
		require.Error(t, err)
		require.Contains(t, err.Error(), "互斥")
	})

	t.Run("两边都不给 ⇒ 报错并给出两种用法", func(t *testing.T) {
		_, err := ResolvePlan(0, 0, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--epochs-file")
		require.Contains(t, err.Error(), "--start")
	})

	t.Run("只给一半区间 ⇒ 沿用区间校验的精确报错", func(t *testing.T) {
		_, err := ResolvePlan(6280001, 0, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "截止高度(--end)必须 > 0")

		_, err = ResolvePlan(6408647, 6357058, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "不能小于起始高度")
	})

	t.Run("清单文件读不到 / 为空 ⇒ 报错（不静默降级为空跑）", func(t *testing.T) {
		_, err := ResolvePlan(0, 0, filepath.Join(dir, "nope.list"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "读取高度清单")

		_, err = ResolvePlan(0, 0, emptyPath)
		require.Error(t, err)
		require.Contains(t, err.Error(), "没有解析出任何高度")
	})
}
