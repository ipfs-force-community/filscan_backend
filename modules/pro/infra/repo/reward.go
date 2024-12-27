package prorepo

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type RewardRepo interface {
	GetMinerRewards(ctx context.Context, miners []string, epochs chain.LCRORange) (items []MinerReward, err error)
}

type MinerReward struct {
	Miner      string
	Reward     decimal.Decimal
	BlockCount int64
	WinCount   int64
	AccReward  decimal.Decimal
}
