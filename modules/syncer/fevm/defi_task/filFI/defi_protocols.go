package filFI

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type FilFi struct {
}

func (s FilFi) GetProtocolName() string {
	return "FilFi"
}

func (s FilFi) GetContractId() string {
	return ""
}

func (s FilFi) GetIconUrl() string {
	return defi_protocols.OssPrefix + "filfi.png"
}

func (s FilFi) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("filfi")
}

func (s FilFi) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	return 0, nil
}

var _ defi_protocols.DefiProtocols = (*FilFi)(nil)
