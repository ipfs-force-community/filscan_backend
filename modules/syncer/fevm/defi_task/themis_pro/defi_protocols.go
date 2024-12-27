package themis_pro

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type ThemisPro struct {
}

func (s ThemisPro) GetProtocolName() string {
	return "Themis Pro"
}

func (s ThemisPro) GetContractId() string {
	return "0xc8e4ef1148d11f8c557f677ee3c73901cd796bf6"
}

func (s ThemisPro) GetIconUrl() string {
	return defi_protocols.OssPrefix + "themis-pro.png"
}

func (s ThemisPro) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilamaThs()
}

func (s ThemisPro) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0x71bf13fa629eef06df8400b2a7c78e204d9f4b63")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*ThemisPro)(nil)
