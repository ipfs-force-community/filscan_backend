package po

type MinerWinCount struct {
	Epoch    int64
	Miner    string
	WinCount int64
}

func (MinerWinCount) TableName() string {
	return "chain.miner_win_counts"
}
