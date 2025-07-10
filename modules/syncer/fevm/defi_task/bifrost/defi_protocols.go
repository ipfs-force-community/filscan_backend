package bifrost

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type BifrostLiquidStaking struct {
}

func (s BifrostLiquidStaking) GetProtocolName() string {
	return "BifrostLiquidStaking"
}

// TODO: no information
func (s BifrostLiquidStaking) GetContractId() string {
	return "0xcdafc875eed0f346d5d2584fc6c37b539459c445"
}

func (s BifrostLiquidStaking) GetIconUrl() string {
	return defi_protocols.OssPrefix + "Bio.png"
}

func (s BifrostLiquidStaking) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("bifrost-liquid-staking")
}

func (s BifrostLiquidStaking) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	return 0, nil
}

var _ defi_protocols.DefiProtocols = (*BifrostLiquidStaking)(nil)
