package po

type SyncSyncer struct {
	Name  string
	Epoch int64
}

func (SyncSyncer) TableName() string {
	return "chain.sync_syncers"
}
