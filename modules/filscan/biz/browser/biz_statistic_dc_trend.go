package browser

import (
	"context"
	"sort"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewStatisticDcTrendBiz(se repository.SyncerGetter, repo repository.StatisticDcTrendBizRepo) *StatisticDcTrendBiz {
	return &StatisticDcTrendBiz{se: se, repo: repo}
}

var _ filscan.StatisticDCTrend = (*StatisticDcTrendBiz)(nil)

type StatisticDcTrendBiz struct {
	se   repository.SyncerGetter
	repo repository.StatisticDcTrendBizRepo
}

func (s StatisticDcTrendBiz) DCTrend(ctx context.Context, req filscan.DCTrendRequest) (resp filscan.DCTrendResponse, err error) {
	current, err := s.se.GetSyncer(ctx, syncer.ChainSyncer)
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

	var points []int64
	for _, v := range intervalPoints.Points() {
		points = append(points, v.Int64())
	}

	r, err := s.repo.QueryDCPowers(ctx, points)
	if err != nil {
		return
	}

	resp.Epoch = current.Epoch
	resp.BlockTime = chain.Epoch(current.Epoch).Time().Unix()

	for _, v := range r {
		resp.Items = append(resp.Items, &filscan.DCTrendItem{
			Epoch:     v.Epoch,
			BlockTime: chain.Epoch(v.Epoch).Time().Unix(),
			Dc:        v.Dc,
			Cc:        v.Cc,
		})
	}

	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].BlockTime < resp.Items[j].BlockTime
	})

	return
}
