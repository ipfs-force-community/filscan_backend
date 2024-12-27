package mineFi

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type MineFi struct {
}

func (s MineFi) GetProtocolName() string {
	return "MineFi"
}

func (s MineFi) GetContractId() string {
	return "0xeee7482aad18794fb7318d9ba694369d16ffbf7b"
}

func (s MineFi) GetIconUrl() string {
	return defi_protocols.OssPrefix + "MineFi.png"
}

func (s MineFi) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("minefi")
}

// TODO: fix nft
func (s MineFi) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	return 0, nil
}

var _ defi_protocols.DefiProtocols = (*MineFi)(nil)
