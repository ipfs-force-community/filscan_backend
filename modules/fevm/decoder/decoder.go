package decoder

import (
	"github.com/ethereum/go-ethereum/ethclient"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gorm.io/gorm"
)

func NewDecoder(db *gorm.DB, client *ethclient.Client) *Decoder {
	contractDecoder := NewContractDecoder(db, client)
	return &Decoder{
		ContractDecoder: contractDecoder,
		FNS:             NewFNS(client),
	}
}

var _ fevm.ABIDecoderAPI = (*Decoder)(nil)

type Decoder struct {
	*ContractDecoder
	*FNS
}
