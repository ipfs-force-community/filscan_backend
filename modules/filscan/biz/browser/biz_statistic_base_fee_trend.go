package browser

import (
	"context"
	"sort"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewBaseFeeTrendBiz(se repository.SyncerGetter, repo repository.BaseFeeTrendBizRepo) *BaseFeeTrendBiz {
	return &BaseFeeTrendBiz{se: se, repo: repo}
}

var _ filscan.StatisticBaseFeeTrend = (*BaseFeeTrendBiz)(nil)

type BaseFeeTrendBiz struct {
	se   repository.SyncerGetter
	repo repository.BaseFeeTrendBizRepo
}

func (b BaseFeeTrendBiz) BaseFeeTrend(ctx context.Context, req filscan.BaseFeeTrendRequest) (resp *filscan.BaseFeeTrendResponse, err error) {
	current, err := b.se.GetSyncer(ctx, syncer.ChainSyncer)
	if err != nil {
		return
	}
	if current == nil {
		return
	}

	var intervalPoints interval.Interval
	intervalPoints, err = interval.ResolveInterval(req.Interval, chain.Epoch(current.Epoch))
	if err != nil {
		return
	}
	var gasCosts []*stat.BaseGasCost
	gasCosts, err = b.repo.GetStatBaseGasCost(ctx, intervalPoints.Points())
	if err != nil {
		return
	}
	if gasCosts != nil {
		a := assembler.BaseFeeTrendAssembler{}
		resp, err = a.ToBaseFeeTrendResponse(chain.Epoch(current.Epoch), gasCosts)
		if err != nil {
			return
		}
	}
	sort.Slice(resp.List, func(i, j int) bool {
		return resp.List[i].Timestamp < resp.List[j].Timestamp
	})
	return
}
