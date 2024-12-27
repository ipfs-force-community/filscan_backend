package filetFi

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type FiletFi struct {
}

func (s FiletFi) GetProtocolName() string {
	return "Filet Finance"
}

func (s FiletFi) GetContractId() string {
	return "0x01502cae9e6f973eab687aa99ba1b332aaa1837f"
}

func (s FiletFi) GetIconUrl() string {
	return defi_protocols.OssPrefix + "Filet.png"
}

func (s FiletFi) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("filet-finance")
}

func (s FiletFi) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0xe12629ce990377f5e6a381e0805f7d86b0681ec9")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*FiletFi)(nil)
