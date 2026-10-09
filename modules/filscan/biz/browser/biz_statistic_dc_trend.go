package browser

import (
	"context"
	"sort"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
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
	// 本网 NV29 激活高度；未排期＝0。前端据此画 NV29 竖线。
	resp.Nv29Epoch = nv29EpochOrZero(message_detail.UpgradeSolsticeHeight.Int64())

	// 链上真值（raw / QA）透传；两档倍数由 chain.QualityTierSplit 在此派生，
	// 保证「一个公式一处实现」：前端只渲染 full_multiplier_power / pending_upgrade_power。
	for _, v := range r {
		full, pending := chain.QualityTierSplit(v.QualityAdjPower, v.RawBytePower)
		resp.Items = append(resp.Items, &filscan.DCTrendItem{
			Epoch:               v.Epoch,
			BlockTime:           chain.Epoch(v.Epoch).Time().Unix(),
			Raw:                 v.RawBytePower,
			QualityAdjPower:     v.QualityAdjPower,
			FullMultiplierPower: full,
			PendingUpgradePower: pending,
		})
	}

	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].BlockTime < resp.Items[j].BlockTime
	})

	return
}
