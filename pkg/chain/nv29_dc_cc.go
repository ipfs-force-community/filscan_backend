package chain

import "github.com/shopspring/decimal"

// DcCcSplit 把全网（f04 状态里的）TotalQualityAdjPower / TotalRawBytePower 拆成 DC 与 CC 两条曲线。
//
// 口径在 NV29（Solstice / FIP-0118）处切换，历史不重算：
//
//   - epoch < solsticeEpoch（datacap 时代）：dc = (QA − raw) / 9，cc = raw − dc。
//     当时每个 verified 字节额外给 9 倍，所以 (QA−raw)/9 恰好等于被 datacap 支撑的算力
//     （也就是「验证算力 / DC」），其余是容量算力（CC）。
//
//   - epoch >= solsticeEpoch：dc = 0，cc = raw。
//     FIP-0118 冻结了 verifreg/datacap 的所有写入路径，网络里不再存在「有验证交易支撑」的算力；
//     QA 超出 raw 的部分全部来自带 FULL_QA_POWER 标志的扇区的 10 倍率（新扇区 + method 37
//     升级过的老扇区），与 verified deal 无关。若继续用 (QA−raw)/9 当 DC，会把「10 倍率带来的
//     质量增益」误当成 datacap 算力（calibnet 实测该项 ≈ 全部新增算力）。
//
// 第三个返回值 fullQaPower 是「质量增益折算成字节」的量（post 口径下 = (QA−raw)/9，即 10 倍率部分），
// 供将来单独展示用；pre 口径下返回 0（该量在旧口径里就等于 dc）。
//
// solsticeEpoch <= 0（主网尚未排期 NV29 时该常量是 UpgradeHeightUnscheduled）时一律走旧口径。
func DcCcSplit(epoch int64, solsticeEpoch int64, qa, raw decimal.Decimal) (dc, cc, fullQaPower decimal.Decimal) {
	nine := decimal.NewFromInt(9)
	if solsticeEpoch > 0 && epoch >= solsticeEpoch {
		return decimal.Zero, raw, qa.Sub(raw).Div(nine)
	}
	dc = qa.Sub(raw).Div(nine)
	cc = raw.Sub(dc)
	return dc, cc, decimal.Zero
}
