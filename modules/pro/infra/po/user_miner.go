package propo

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"time"
)

type UserMiner struct {
	UserID    int64
	GroupID   *int64
	MinerID   chain.SmartAddress
	MinerTag  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (UserMiner) TableName() string {
	return "pro.user_miners"
}
