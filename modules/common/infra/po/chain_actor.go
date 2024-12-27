package po

import (
	"time"

	"github.com/shopspring/decimal"
)

type ActorPo struct {
	Id          string
	Robust      *string
	Type        string
	Code        string
	CreatedTime *time.Time
	LastTxTime  *time.Time
	Balance     decimal.Decimal
}

func (ActorPo) TableName() string {
	return "chain.actors"
}

type ActorChangePo struct {
	Epoch int64
	ActorPo
}

func (ActorChangePo) TableName() string {
	return "chain.actors_change"
}
