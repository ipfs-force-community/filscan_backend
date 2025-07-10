package po

import "github.com/shopspring/decimal"

type BuiltinActorStatePo struct {
	Epoch   int64
	Actor   string
	Balance decimal.Decimal
	State   string
}

func (BuiltinActorStatePo) TableName() string {
	return "chain.builtin_actor_states"
}
