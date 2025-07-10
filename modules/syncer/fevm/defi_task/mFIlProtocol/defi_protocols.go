package mFIlProtocol

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
)

type MFIL struct {
}

func (s MFIL) GetProtocolName() string {
	return "MFIL Protocol"
}

func (s MFIL) GetContractId() string {
	return "0x8af827cda3b7eee9720496a30595d7ee89a27ee2"
}

func (s MFIL) GetIconUrl() string {
	return defi_protocols.OssPrefix + "MFL.png"
}

func (s MFIL) GetTvl() (defi_protocols.Tvl, error) {
	return defi_protocols.GetTvlFromDefilama("mfil-protocol")
}

func (s MFIL) GetUsers(repo repository.ERC20TokenRepo) (int, error) {
	res, err := repo.GetUniqueNoneZeroTokenHolderByContract(context.TODO(), "0x8af827cda3b7eee9720496a30595d7ee89a27ee2")
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

var _ defi_protocols.DefiProtocols = (*MFIL)(nil)
