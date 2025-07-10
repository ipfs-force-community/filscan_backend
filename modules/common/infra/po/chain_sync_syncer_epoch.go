package po

import "gitlab.forceup.in/fil-data-factory/filscan-backend/types"

type SyncSyncerEpoch struct {
	Epoch      int64
	Name       string
	Keys       types.StringArray
	ParentKeys types.StringArray
	Cost       int64
	Empty      bool
}

func (SyncSyncerEpoch) TableName() string {
	return "chain.sync_syncer_epochs"
}
