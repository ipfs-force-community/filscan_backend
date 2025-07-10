package browser

import (
	"context"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewStatisticMessageCountTrendBiz(se repository.SyncerGetter, repo repository.StatisticMessageCountTrendBizRepo) *StatisticMessageCountTrendBiz {
	return &StatisticMessageCountTrendBiz{se: se, repo: repo}
}

var _ filscan.StatisticMessageCountTrend = (*StatisticMessageCountTrendBiz)(nil)

type StatisticMessageCountTrendBiz struct {
	se   repository.SyncerGetter
	repo repository.StatisticMessageCountTrendBizRepo
}

func (s StatisticMessageCountTrendBiz) MessageCountTrend(ctx context.Context, req filscan.MessageCountTrendRequest) (resp *filscan.MessageCountTrendResponse, err error) {
	epoch, err := s.se.GetSyncer(context.Background(), syncer.ChainSyncer)
	if err != nil {
		return
	}

	if epoch != nil {
		var it interval.Interval
		it, err = interval.ResolveInterval(req.Interval, chain.Epoch(epoch.Epoch))
		if err != nil {
			return
		}

		var items []*bo.MessageCount
		items, err = s.repo.GetMessageCountsByEpochs(ctx, it.Points())
		if err != nil {
			return
		}

		a := assembler.MessageCountTrendAssembler{}

		resp, err = a.ToMessageCountTrendResponse(chain.Epoch(epoch.Epoch), items)
		if err != nil {
			return
		}
	}

	return
}
