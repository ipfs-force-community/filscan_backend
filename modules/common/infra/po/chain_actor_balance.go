package po

import "github.com/shopspring/decimal"

type ActorBalance struct {
	Epoch     int64
	ActorId   string
	ActorType *string
	Balance   decimal.Decimal
	PrevEpoch int64
}

func (ActorBalance) TableName() string {
	return "chain.actor_balances"
}
