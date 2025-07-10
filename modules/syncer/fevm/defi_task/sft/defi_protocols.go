package sft

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type SFT struct {
}

func (s SFT) GetProtocolName() string {
	return "SFT Protocol"
}

func (s SFT) GetContractId() string {
	return "0xcdafc875eed0f346d5d2584fc6c37b539459c445"
}

func (s SFT) GetIconUrl() string {
	return defi_protocols.OssPrefix + "SFT.png"
}

func (s SFT) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("sft-protocol")
}

func (s SFT) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0xc5ea96dd365983cfec90e72b6a2dac9562f458ba")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*SFT)(nil)
