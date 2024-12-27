package actor

import "github.com/shopspring/decimal"

type RichActor struct {
	Epoch     int64
	ActorID   string
	ActorType string
	Balance   decimal.Decimal
}

func (RichActor) TableName() string {
	return "chain.rich_actors"
}
