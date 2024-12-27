package hashmix

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type HashMix struct {
}

func (s HashMix) GetProtocolName() string {
	return "HashMix FIL"
}

func (s HashMix) GetContractId() string {
	return "0x587a7eae9b461ad724391aa7195210e0547ed11d"
}

func (s HashMix) GetIconUrl() string {
	return defi_protocols.OssPrefix + "HSM.png"
}

func (s HashMix) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("hashmix-fil")
}

func (s HashMix) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0x587a7eae9b461ad724391aa7195210e0547ed11d")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*HashMix)(nil)
