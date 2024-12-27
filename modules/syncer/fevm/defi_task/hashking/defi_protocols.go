package hashking

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type HashKing struct {
}

func (s HashKing) GetProtocolName() string {
	return "HashKing"
}

func (s HashKing) GetContractId() string {
	return "0xfeb16a48dbbb0e637f68215b19b4df5b12449676"
}

func (s HashKing) GetIconUrl() string {
	return defi_protocols.OssPrefix + "HASHKING.svg"
}

func (s HashKing) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("hashking")
}

func (s HashKing) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0x84b038db0fcde4fae528108603c7376695dc217f")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*HashKing)(nil)
