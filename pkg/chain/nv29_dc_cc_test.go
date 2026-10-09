package chain

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// QualityTierSplit 唯一口径（NV29/FIP-0118 方案 A）：
//
//	full    = (qa − raw) / 9
//	pending = raw − full
//
// 覆盖满倍率 / 零倍率 / 对称三种边界，并固化「不依赖 epoch（签名里就没有）」「历史不重算」。
func TestQualityTierSplit(t *testing.T) {
	// qa = 10×raw：全部处于 10× 档 ⇒ full = raw、pending = 0
	raw := decimal.NewFromInt(1000)
	qa := decimal.NewFromInt(10000)
	full, pending := QualityTierSplit(qa, raw)
	require.True(t, full.Equal(raw), "qa=10raw: full 应等于 raw, got %s", full)
	require.True(t, pending.Equal(decimal.Zero), "qa=10raw: pending 应为 0, got %s", pending)

	// qa = raw：无任何质量增益 ⇒ full = 0、pending = raw
	full, pending = QualityTierSplit(raw, raw)
	require.True(t, full.Equal(decimal.Zero), "qa=raw: full 应为 0, got %s", full)
	require.True(t, pending.Equal(raw), "qa=raw: pending 应等于 raw, got %s", pending)

	// qa = 5.5×raw：对称点 ⇒ full = pending = 0.5×raw
	// （旧口径下这正是「一半 DC、一半 CC」，改造后两条线在此点重合）
	half := decimal.NewFromInt(500)
	full, pending = QualityTierSplit(decimal.NewFromInt(5500), raw)
	require.True(t, full.Equal(half), "qa=5.5raw: full 应为 0.5raw, got %s", full)
	require.True(t, pending.Equal(half), "qa=5.5raw: pending 应为 0.5raw, got %s", pending)
	require.True(t, full.Equal(pending), "qa=5.5raw: 两档应对称相等")

	// 不变量：full + pending 恒等于 raw（无字节丢失）
	require.True(t, full.Add(pending).Equal(raw), "full+pending 应恒等于 raw, got %s", full.Add(pending))

	// 不依赖 epoch：签名中已无 epoch，同输入必同输出（显式固化）
	f1, p1 := QualityTierSplit(qa, raw)
	f2, p2 := QualityTierSplit(qa, raw)
	require.True(t, f1.Equal(f2) && p1.Equal(p2), "同输入必须同输出")

	// 历史不重算：NV29 前旧口径 (dc, cc) 恰为 (full, pending)
	dcOld := qa.Sub(raw).Div(decimal.NewFromInt(9))
	ccOld := raw.Sub(dcOld)
	f3, p3 := QualityTierSplit(qa, raw)
	require.True(t, f3.Equal(dcOld), "full 应等于旧 DC（历史不重算）, got %s", f3)
	require.True(t, p3.Equal(ccOld), "pending 应等于旧 CC（历史不重算）, got %s", p3)
}
