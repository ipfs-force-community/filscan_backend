package browser

import (
	"context"
	"sort"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewStatisticBlockRewardTrendBiz(se repository.SyncerGetter, repo repository.StatisticBlockRewardTrendBizRepo) *StatisticBlockRewardTrendBiz {
	return &StatisticBlockRewardTrendBiz{se: se, repo: repo}
}

var _ filscan.StatisticBlockRewardTrend = (*StatisticBlockRewardTrendBiz)(nil)

type StatisticBlockRewardTrendBiz struct {
	se   repository.SyncerGetter
	repo repository.StatisticBlockRewardTrendBizRepo
}

func (s StatisticBlockRewardTrendBiz) BlockRewardTrend(ctx context.Context, req filscan.BlockRewardTrendRequest) (resp *filscan.BlockRewardTrendResponse, err error) {

	epoch, err := s.se.GetSyncer(context.Background(), syncer.ChainSyncer)
	if err != nil {
		return
	}

	if epoch == nil {
		return
	}

	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, chain.Epoch(epoch.Epoch))
	if err != nil {
		return
	}

	var points []int64
	for _, v := range it.Points() {
		points = append(points, v.Int64())
	}

	var items []*bo.SumMinerReward
	items, err = s.repo.GetBlockRewardsByEpochs(ctx, "24h", points)
	if err != nil {
		return
	}

	a := assembler.BlockRewardTrendAssembler{}

	resp, err = a.ToBlockRewardTrendResponse(chain.Epoch(epoch.Epoch), items)
	if err != nil {
		return
	}
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].BlockTime < resp.Items[j].BlockTime
	})
	return
}
