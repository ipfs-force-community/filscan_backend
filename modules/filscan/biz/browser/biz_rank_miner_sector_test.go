package browser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归：前端「全部」档位传 "all"，此前会走 humanize.ParseBytes 报错 → 接口 500 → 页面 No data。
func TestParseRankSectorSize(t *testing.T) {
	cases := []struct {
		in      string
		want    uint64
		wantErr bool
	}{
		{"all", 0, false},   // 前端「全部」
		{"ALL", 0, false},   // 大小写不敏感
		{" all ", 0, false}, // 带空白
		{"", 0, false},      // 前端也可能传空串
		{"32GiB", 32 << 30, false},
		{"64GiB", 64 << 30, false},
		{"not-a-size", 0, true}, // 真正非法的值仍应报错（不要静默吞掉）
	}
	for _, c := range cases {
		got, err := parseRankSectorSize(c.in)
		if c.wantErr {
			require.Error(t, err, "input=%q 应该报错", c.in)
			continue
		}
		require.NoError(t, err, "input=%q 不应报错", c.in)
		require.Equal(t, c.want, got, "input=%q", c.in)
	}
}
