package po

import "time"

type EvmEventSignature struct {
	ID            int64
	TextSignature string
	HexSignature  string
	CreatedAt     time.Time
}

func (e EvmEventSignature) TableName() string {
	return "fevm.evm_event_signatures"
}
