package po

import (
	"github.com/shopspring/decimal"
	"time"
)

type MinerRewardPo struct {
	Epoch         int64
	BlockTime     time.Time
	Miner         string
	Reward        decimal.Decimal
	BlockCount    int64
	AccReward     decimal.Decimal
	AccBlockCount int64
	//PrevRewardRef int64
}

func (MinerRewardPo) TableName() string {
	return "chain.miner_rewards"
}
