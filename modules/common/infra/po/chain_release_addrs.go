package po

type ReleaseAddrs struct {
	Address      string  `gorm:"column:address"`
	Tag          string  `gorm:"column:tag"`
	DailyRelease float64 `gorm:"column:daily_release"`
	StartEpoch   int64   `gorm:"column:start_epoch"`
	EndEpoch     int64   `gorm:"column:end_epoch"`
	InitialLock  float64 `gorm:"column:initial_lock"`
}

func (ReleaseAddrs) TableName() string {
	return "chain.release_addrs"
}
