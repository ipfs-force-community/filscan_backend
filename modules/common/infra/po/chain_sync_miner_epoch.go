package po

type SyncMinerEpochPo struct {
	Epoch           int64
	EffectiveMiners int64
	Owners          int64
}

func (SyncMinerEpochPo) TableName() string {
	return "chain.sync_miner_epochs"
}
