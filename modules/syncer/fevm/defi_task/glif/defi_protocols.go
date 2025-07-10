package glif

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type Glif struct {
}

func (s Glif) GetProtocolName() string {
	return "GLIF Pools Infinity Pool"
}

func (s Glif) GetContractId() string {
	return "0x43dae5624445e7679d16a63211c5ff368681500c"
}

func (s Glif) GetIconUrl() string {
	return defi_protocols.OssPrefix + "GLIF.png"
}

func (s Glif) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("glif")
}

func (s Glif) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0x690908f7fa93afc040cfbd9fe1ddd2c2668aa0e0")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*Glif)(nil)
