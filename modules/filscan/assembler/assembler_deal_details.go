package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type DealDetailsAssembler struct {
}

func (DealDetailsAssembler) ToDealDetailsResponse(detail *londobell.Deals, cid string) (target filscan.DealDetailsResponse) {
	target.DealDetails = &filscan.DealDetails{
		DealID:               detail.ID,
		Epoch:                detail.Epoch,
		MessageCid:           cid,
		PieceCid:             detail.PieceCID,
		VerifiedDeal:         detail.VerifiedDeal,
		PieceSize:            detail.PieceSize,
		Client:               detail.Client.Address(),
		ClientCollateral:     detail.ClientCollateral,
		Provider:             detail.Provider.Address(),
		ProviderCollateral:   detail.ProviderCollateral,
		StartEpoch:           detail.StartEpoch,
		StartTime:            chain.Epoch(detail.StartEpoch).Unix(),
		EndEpoch:             detail.EndEpoch,
		EndTime:              chain.Epoch(detail.EndEpoch).Unix(),
		StoragePricePerEpoch: detail.StoragePricePerEpoch,
		Label:                detail.Label,
	}

	return
}
