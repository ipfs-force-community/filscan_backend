package filliquid

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type FILLiquid struct {
}

func (s FILLiquid) GetProtocolName() string {
	return "FILLiquid"
}

func (s FILLiquid) GetContractId() string {
	return "0xfb1473ba128B9c8146C899cf9455c0037631D389"
}

func (s FILLiquid) GetIconUrl() string {
	return defi_protocols.OssPrefix + "FILLiquid.png"
}

func (s FILLiquid) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("filliquid")
}

func (s FILLiquid) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0xfd669bddfbb0d085135cbd92521785c39c95ba4b")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*FILLiquid)(nil)
