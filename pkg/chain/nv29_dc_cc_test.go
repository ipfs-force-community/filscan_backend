package chain

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

const testSolstice = int64(4109133)

func TestDcCcSplit(t *testing.T) {
	raw := decimal.NewFromInt(1000)
	qa := decimal.NewFromInt(10000) // QA 是 raw 的 10 倍（calibnet 现状：绝大多数扇区满 QA）

	// 升级前一高度：老口径，(QA-raw)/9 = 1000 → dc=1000、cc=0
	dc, cc, full := DcCcSplit(testSolstice-1, testSolstice, qa, raw)
	require.True(t, dc.Equal(decimal.NewFromInt(1000)), "pre: dc 应为 (qa-raw)/9, got %s", dc)
	require.True(t, cc.Equal(decimal.Zero), "pre: cc = raw-dc = 0, got %s", cc)
	require.True(t, full.Equal(decimal.Zero), "pre: 不额外返回质量增益")

	// 升级高度本身：新口径，dc=0、cc=raw，质量增益单独返回
	dc, cc, full = DcCcSplit(testSolstice, testSolstice, qa, raw)
	require.True(t, dc.Equal(decimal.Zero), "post: dc 必须为 0（FIL+ 已冻结），got %s", dc)
	require.True(t, cc.Equal(raw), "post: cc = raw（全部为容量算力），got %s", cc)
	require.True(t, full.Equal(decimal.NewFromInt(1000)), "post: 质量增益 (qa-raw)/9, got %s", full)

	// 主网未排期 NV29：solsticeEpoch 为 0（unscheduled）时必须走旧口径
	dc, _, _ = DcCcSplit(6_411_000, 0, qa, raw)
	require.True(t, dc.Equal(decimal.NewFromInt(1000)), "unscheduled: 应走旧口径, got %s", dc)

	// 边界：QA == raw（无任何质量增益）时两个口径都给 cc=raw、dc=0
	for _, ep := range []int64{testSolstice - 1, testSolstice} {
		dc, cc, _ = DcCcSplit(ep, testSolstice, raw, raw)
		require.True(t, dc.Equal(decimal.Zero), "epoch=%d 无增益时 dc=0", ep)
		require.True(t, cc.Equal(raw), "epoch=%d 无增益时 cc=raw", ep)
	}
}
