package chain

import "github.com/shopspring/decimal"

// QualityTierSplit 把任一时间点全网的（有效算力 qa、原始算力 raw）拆成两档「倍数」口径：
//
//	full    = (qa − raw) / 9   满倍率算力：处于 10× 档的等效原始字节
//	pending = raw − full       待升级算力：未达满倍率的等效原始字节
//	                           （可通过 snap / UpgradeSectorQuality 升级到满倍率）
//
// 两个时代同式：NV29（Solstice / FIP-0118）之前 full 恰等于旧的 DC（有验证交易支撑的字节）、
// pending 恰等于旧的 CC；NV29 之后 FIL+ 冻结、新扇区一律 10×，同一公式描述的只是「字节处于哪一档
// 倍数」，与内容无关 ⇒ 历史不重算、曲线连续。因此本函数不接收 epoch，也不存在任何 epoch 分支。
//
// 注意：本口径回答「字节处于哪一档倍数」，与 pkg/londobell.QASplit 的逐扇区三桶（VDC/DC/CC，QA 口径，
// 写 pro.miner_dcs）不是同一口径，两处词表不得混用。
func QualityTierSplit(qa, raw decimal.Decimal) (full, pending decimal.Decimal) {
	full = qa.Sub(raw).Div(decimal.NewFromInt(9))
	pending = raw.Sub(full)
	return
}
