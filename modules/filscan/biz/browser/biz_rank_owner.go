package browser

import (
	"context"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
)

func NewOwnerRankBiz(se repository.SyncEpochGetter, repo repository.OwnerRankBizRepo) *OwnerRankBiz {
	return &OwnerRankBiz{se: se, repo: repo}
}

var _ filscan.OwnerRankAPI = (*OwnerRankBiz)(nil)

type OwnerRankBiz struct {
	se   repository.SyncEpochGetter
	repo repository.OwnerRankBizRepo
}

func (o OwnerRankBiz) OwnerRank(ctx context.Context, query filscan.PagingQuery) (resp *filscan.OwnerRankResponse, err error) {
	err = query.Valid()
	if err != nil {
		return
	}
	
	epoch, err := o.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	
	if epoch == nil {
		return
	}
	
	var owners []*bo.OwnerRank
	var total int64
	owners, total, err = o.repo.GetOwnerRanks(ctx, *epoch, query)
	if err != nil {
		return
	}
	
	a := assembler.OwnerRankAssembler{}
	
	resp, err = a.ToOwnerRankResponse(total, query.Index, query.Limit, owners)
	if err != nil {
		return
	}
	resp.UpdatedAt = epoch.Time().Unix()
	
	return
}
