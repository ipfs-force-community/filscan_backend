package browser

import (
	"context"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
)

func NewStatisticActiveMinerTrendBiz(se repository.SyncEpochGetter, repo repository.StatisticActiveMinerTrendBizRepo) *StatisticActiveMinerTrendBiz {
	return &StatisticActiveMinerTrendBiz{se: se, repo: repo}
}

var _ filscan.StatisticActiveMinerTrend = (*StatisticActiveMinerTrendBiz)(nil)

type StatisticActiveMinerTrendBiz struct {
	se   repository.SyncEpochGetter
	repo repository.StatisticActiveMinerTrendBizRepo
}

func (s StatisticActiveMinerTrendBiz) ActiveMinerTrend(ctx context.Context, req filscan.ActiveMinerTrendRequest) (resp *filscan.ActiveMinerTrendResponse, err error) {
	
	epoch, err := s.se.MinerEpoch(context.Background())
	if err != nil {
		return
	}
	
	if epoch == nil {
		return
	}
	
	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, *epoch)
	if err != nil {
		return
	}
	
	var items []*bo.ActiveMinerCount
	items, err = s.repo.GetActiveMinerCountsByEpochs(ctx, it.Points())
	if err != nil {
		return
	}
	
	a := assembler.ActiveMinerTrendAssembler{}
	
	resp, err = a.ToActiveMinerTrendResponse(*epoch, items)
	if err != nil {
		return
	}
	
	return
}
