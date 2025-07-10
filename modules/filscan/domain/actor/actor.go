package actor

import (
	"github.com/filecoin-project/go-state-types/network"
	"github.com/ipfs/go-cid"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type Id = chain.SmartAddress
type Robust = chain.SmartAddress

type Actor struct {
	Network network.Version
	Id      Id
	Robust  Robust
	Type    string
	Code    cid.Cid
}
