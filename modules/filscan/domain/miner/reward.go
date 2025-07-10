package miner

import "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"

type Reward struct {
	Epoch      chain.Epoch
	Miner      chain.SmartAddress
	Reward     chain.AttoFil
	BlockCount int64
}
