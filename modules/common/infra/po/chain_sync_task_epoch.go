package po

type SyncTaskEpoch struct {
	Epoch  int64
	Task   string
	Syncer string
	Cost   int64
}

func (SyncTaskEpoch) TableName() string {
	return "chain.sync_task_epochs"
}
