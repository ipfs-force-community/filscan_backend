package metrics

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetRenderFormat(t *testing.T) {
	set := NewSet()
	require.NoError(t, set.Add("filscan_demo_height", "演示指标\n单位：高度", Gauge,
		map[string]string{"syncer": "chain", "network": "mainnet"}, 6357057))
	require.NoError(t, set.Add("filscan_demo_height", "演示指标\n单位：高度", Gauge,
		map[string]string{"syncer": "miner", "network": "mainnet"}, 60))
	require.NoError(t, set.Add("filscan_demo_total", "演示计数", Counter, nil, 3))

	out := string(set.Bytes())
	require.Equal(t,
		"# HELP filscan_demo_height 演示指标\\n单位：高度\n"+
			"# TYPE filscan_demo_height gauge\n"+
			"filscan_demo_height{network=\"mainnet\",syncer=\"chain\"} 6357057\n"+
			"filscan_demo_height{network=\"mainnet\",syncer=\"miner\"} 60\n"+
			"# HELP filscan_demo_total 演示计数\n"+
			"# TYPE filscan_demo_total counter\n"+
			"filscan_demo_total 3\n",
		out)
}

func TestSetRenderSortsSamplesByLabels(t *testing.T) {
	set := NewSet()
	require.NoError(t, set.Add("m", "h", Gauge, map[string]string{"b": "2"}, 1))
	require.NoError(t, set.Add("m", "h", Gauge, map[string]string{"a": "1"}, 2))
	require.NoError(t, set.Add("m", "h", Gauge, map[string]string{"a": "0"}, 3))
	out := string(set.Bytes())
	require.Equal(t, "# HELP m h\n# TYPE m gauge\nm{a=\"0\"} 3\nm{a=\"1\"} 2\nm{b=\"2\"} 1\n", out)
}

func TestSetRenderEscapesLabelsAndHelp(t *testing.T) {
	set := NewSet()
	require.NoError(t, set.Add("m", "反斜杠 \\ 与换行\n结尾", Gauge,
		map[string]string{"q": "a\"b\\c\nd"}, 1))
	out := string(set.Bytes())
	require.Equal(t, "# HELP m 反斜杠 \\\\ 与换行\\n结尾\n# TYPE m gauge\nm{q=\"a\\\"b\\\\c\\nd\"} 1\n", out)
}

func TestSetRenderRejectsConflictingHelpOrType(t *testing.T) {
	set := NewSet()
	require.NoError(t, set.Add("m", "h1", Gauge, nil, 1))
	require.Error(t, set.Add("m", "h2", Gauge, nil, 2))
	require.Error(t, set.Add("m", "h1", Counter, nil, 2))
}

func TestSetRenderDropsDuplicateLabelSets(t *testing.T) {
	// 同名同标签的重复样本会让 Prometheus 整轮抓取失败，必须去重。
	set := NewSet()
	require.NoError(t, set.Add("m", "h", Gauge, map[string]string{"a": "1"}, 1))
	require.NoError(t, set.Add("m", "h", Gauge, map[string]string{"a": "1"}, 99))
	out := string(set.Bytes())
	require.Equal(t, 1, strings.Count(out, "m{a=\"1\"}"))
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{60, "60"},
		{6357057, "6357057"},
		{-42, "-42"},
		{1.5, "1.5"},
		{0.001, "0.001"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, formatValue(c.in), "formatValue(%v)", c.in)
	}
	require.Equal(t, "NaN", formatValue(math.NaN()))
	require.Equal(t, "+Inf", formatValue(math.Inf(1)))
	require.Equal(t, "-Inf", formatValue(math.Inf(-1)))
}
