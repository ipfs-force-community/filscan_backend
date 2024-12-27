package injector

import (
	"github.com/google/wire"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
)

var NewAbiDecoderSet = wire.NewSet(NewAbiDecoderClient)

func NewAbiDecoderClient(conf *config.Config) (c fevm.ABIDecoderAPI, err error) {
	return fevm.NewAbiDecoderClient(*conf.ABIDecoderRPC)
}
