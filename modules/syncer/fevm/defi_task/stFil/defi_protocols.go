package stFil

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type StakeFil struct {
}

func (s StakeFil) GetProtocolName() string {
	return "stFIL"
}

func (s StakeFil) GetContractId() string {
	return "0xc8e4ef1148d11f8c557f677ee3c73901cd796bf6"
}

func (s StakeFil) GetIconUrl() string {
	return defi_protocols.OssPrefix + "STFIL.png"
}

func (s StakeFil) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("stfil")
}

func (s StakeFil) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0x3c3501e6c353dbaeddfa90376975ce7ace4ac7a8")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*StakeFil)(nil)
