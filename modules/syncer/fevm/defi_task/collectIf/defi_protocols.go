package collectIf

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type Collectif struct {
}

func (s Collectif) GetProtocolName() string {
	return "Collectif DAO"
}

func (s Collectif) GetContractId() string {
	return "0xd0437765d1dc0e2fa14e97d290f135efdf1a8a9a"
}

func (s Collectif) GetIconUrl() string {
	return defi_protocols.OssPrefix + "Collect.png"
}

func (s Collectif) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("collectif-dao")
}

func (s Collectif) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0xd0437765d1dc0e2fa14e97d290f135efdf1a8a9a")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*Collectif)(nil)
