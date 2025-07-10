package bo

import (
	"github.com/shopspring/decimal"
	"time"
)

type ActorInfo struct {
	Id          string
	Robust      *string
	Type        string
	Code        string
	CreatedTime *time.Time
	LastTxTime  *time.Time
	Balance     decimal.Decimal
}
