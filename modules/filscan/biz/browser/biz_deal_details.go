package browser

import (
	"context"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

func NewDealDetailsBiz(agg londobell.Agg, db *gorm.DB) *DealDetailsBiz {
	return &DealDetailsBiz{
		agg:          agg,
		dealProposal: dal.NewDealProposalTaskDal(db),
	}
}

var _ filscan.DealDetailAPI = (*DealDetailsBiz)(nil)

type DealDetailsBiz struct {
	agg          londobell.Agg
	dealProposal repository.DealProposalTaskRepo
}

func (d DealDetailsBiz) DealDetails(ctx context.Context, req filscan.DealDetailsRequest) (result filscan.DealDetailsResponse, err error) {
	deal, err := d.agg.DealByID(ctx, req.DealID)
	if err != nil {
		return
	}
	if deal != nil {
		var dealProposal *po.DealProposalPo
		dealProposal, err = d.dealProposal.GetCidByDeal(ctx, req.DealID)
		if err != nil {
			return
		}
		var cid string
		if dealProposal != nil {
			cid = dealProposal.Cid
		}
		convertor := assembler.DealDetailsAssembler{}
		result = convertor.ToDealDetailsResponse(deal[0], cid)
		if err != nil {
			return
		}
	}
	
	return
}
