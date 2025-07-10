package injector

import (
	"github.com/go-resty/resty/v2"
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	londobellimpl "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/impl"
)

var LondoBellAggProviderSet = wire.NewSet(NewLondobellAgg, NewLondobellMinerAgg)

func NewLondobellAgg(conf *config.Config) (londobell.Agg, error) {
	return londobellimpl.NewLondobellAggImpl(*conf.Londobell.AggAddress, resty.New()), nil
}

func NewLondobellMinerAgg(conf *config.Config) (londobell.MinerAgg, error) {
	return londobellimpl.NewMinerAggImpl(*conf.Londobell.MinerAggAddress)
}
