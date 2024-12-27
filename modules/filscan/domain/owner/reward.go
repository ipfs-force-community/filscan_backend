package owner

import "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"

type Reward struct {
	Epoch         chain.Epoch
	Owner         chain.SmartAddress
	Reward        chain.AttoFil
	BlockCount    int64
	PrevEpochRef  chain.Epoch
	AccReward     chain.AttoFil
	AccBlockCount int64
	SyncMinerRef  chain.Epoch
	Miners        []chain.SmartAddress
}
