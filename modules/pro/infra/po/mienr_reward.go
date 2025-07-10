package propo

type MinerReward struct {
}

func (MinerReward) TableName() string {
	return "pro.miner_rewards"
}
