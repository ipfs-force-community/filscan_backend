package injector

import (
	"github.com/go-resty/resty/v2"
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	adapter_impl "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/impl"
)

var LondoBellAdapterProviderSet = wire.NewSet(NewLondobellAdapter)

func NewLondobellAdapter(conf *config.Config) (londobell.Adapter, error) {
	return adapter_impl.NewLondobellAdapterImpl(*conf.Londobell.AdapterAddress, resty.New()), nil
}
