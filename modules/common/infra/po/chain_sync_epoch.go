package po

import (
	"github.com/jackc/pgtype"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type SyncEpochPo struct {
	Epoch int64
	Empty bool
	Cost  pgtype.Interval
	Cids  types.StringArray
}

func (SyncEpochPo) TableName() string {
	return "chain.sync_epochs"
}
