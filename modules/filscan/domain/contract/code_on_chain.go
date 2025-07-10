package contract

import "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"

type OnChainActor struct {
	ActorID      string
	ActorAddress string
	EthAddress   string
	InitCode     *londobell.InitCode
}
