package po

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type OwnerRewardPo struct {
	Epoch         int64
	Owner         string
	Reward        decimal.Decimal
	BlockCount    int64
	AccReward     decimal.Decimal
	AccBlockCount int64
	SyncMinerRef  int64
	PrevEpochRef  int64
	Miners        types.StringArray
}

func (OwnerRewardPo) TableName() string {
	return "chain.owner_rewards"
}
