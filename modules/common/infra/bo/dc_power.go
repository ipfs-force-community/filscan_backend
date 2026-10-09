package bo

import "github.com/shopspring/decimal"

// DCPower 现在只承载链上真值（f04 状态里的全网 raw / QA）。
// 名称沿用（与 DCTrend* / QueryDCPowers 命名保持一致，避免无谓改名波及）；
// 算力倍数口径由调用方经 chain.QualityTierSplit 派生，DAL 只透传真值。
type DCPower struct {
	Epoch           int64
	RawBytePower    decimal.Decimal
	QualityAdjPower decimal.Decimal
}
