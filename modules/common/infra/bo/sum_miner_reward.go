package bo

import "github.com/shopspring/decimal"

// SumMinerReward 统计页「累计区块奖励」曲线的单点。
//
// AccBlockRewards = 该 epoch 的 f02 累计「矿工实收」（attoFIL），由 dal_biz_statistic.go 的
// SQLBlockRewardTrend 从 chain.builtin_actor_states.state(jsonb) 直接算出：
//   - v18 及以前：state.TotalStoragePowerReward（历史字段即矿工实收）；
//   - NV29(Solstice) 起：state.TotalMintedReward − TotalBurnMinted − TotalExplicitMinted。
//
// 字段原名 Balance，指的是 f02 账户余额 —— NV29 后 f02 余额的增减不再等于「累计发给矿工的
// 奖励」（服务流留存 f02、销毁股不回 f02），故改名为 AccBlockRewards，避免调用方再误用余额口径。
type SumMinerReward struct {
	Epoch           int64
	AccBlockRewards decimal.Decimal
	AccRewardPerT   decimal.Decimal
}
