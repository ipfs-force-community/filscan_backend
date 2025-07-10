package browser

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewStatisticGasDataTrendBiz(repo repository.Gas24hTrendBizRepo) *StatisticGasDataTrend {
	return &StatisticGasDataTrend{repo: repo}
}

var _ filscan.StatisticGasDataTrend = (*StatisticGasDataTrend)(nil)

type StatisticGasDataTrend struct {
	repo repository.Gas24hTrendBizRepo
}

func (g StatisticGasDataTrend) GasDataTrend(ctx context.Context, req filscan.GasDataTrendRequest) (resp *filscan.GasDataTrendResponse, err error) {
	
	latest, err := g.repo.GetLatestMethodGasCostEpoch(ctx)
	if err != nil {
		return
	}
	
	entities, err := g.repo.GetMethodGasFees(ctx, chain.NewLCRORange(latest.Next()-2880, latest.Next()))
	if err != nil {
		return
	}
	
	a := assembler.GasDataTrendTrendAssembler{}
	resp = a.ToGasDataTrend(latest, entities)
	
	return
}
