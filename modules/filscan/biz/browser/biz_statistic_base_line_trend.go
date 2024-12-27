package browser

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewStatisticBaseLineBiz(syncEpoch repository.SyncerGetter, repo repository.StatisticBaseLineBizRepo, absPower repository.AbsPower,
	adapter londobell.Adapter) *StatisticBaseLineBiz {
	return &StatisticBaseLineBiz{syncEpoch: syncEpoch, repo: repo, adapter: adapter, minerPower: absPower}
}

var _ filscan.StatisticBaseLineTrend = (*StatisticBaseLineBiz)(nil)

// StatisticBaseLineBiz 获取基线和算力走势统计图
// 30d、365d
type StatisticBaseLineBiz struct {
	syncEpoch           repository.SyncerGetter
	repo                repository.StatisticBaseLineBizRepo
	minerPower          repository.AbsPower
	adapter             londobell.Adapter
	latestBaseLineTrend *filscan.BaseLineTrend
}

func (s *StatisticBaseLineBiz) BaseLineTrend(ctx context.Context, req filscan.BaseLineTrendRequest) (resp *filscan.BaseLineTrendResponse, err error) {

	current, err := s.syncEpoch.GetSyncer(ctx, syncer.ChainSyncer)
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

	var entities []*bo.BaseLinePower
	entities, err = s.repo.GetBaseLinePowerByPoints(ctx, intervalPoints.Points())
	if err != nil {
		return
	}

	start, end := int64(-1), int64(-1)
	for i := range intervalPoints.Points() {
		if start == -1 || intervalPoints.Points()[i].Int64() < start {
			start = intervalPoints.Points()[i].Int64()
		}
		if end == -1 || intervalPoints.Points()[i].Int64() > end {
			end = intervalPoints.Points()[i].Int64()
		}
	}

	powerAbs, err := s.minerPower.GetPowerAbs(ctx, start, end)
	if err != nil {
		return
	}
	if entities != nil {
		a := assembler.BaseLinePowerTrendAssembler{}
		resp, err = a.ToBaseLineTrendResponse(chain.Epoch(current.Epoch), entities, powerAbs)
		if err != nil {
			return
		}
	}

	var prev *filscan.BaseLineTrend

	// 此处需要上面查询的 BaseLine 趋势按 Epoch 降序排序
	if len(resp.List) > 0 {
		prev = resp.List[0]
	}
	last := s.getLatestBaselineTrend(ctx, prev)
	if last != nil {
		resp.List = append([]*filscan.BaseLineTrend{last}, resp.List...)
	}
	sort.Slice(resp.List, func(i, j int) bool {
		return resp.List[i].Timestamp < resp.List[j].Timestamp
	})
	return
}

// 获取最新的 BaseLine 趋势，如果内部发生错误，则返回 Nil
func (s *StatisticBaseLineBiz) getLatestBaselineTrend(ctx context.Context, prev *filscan.BaseLineTrend) *filscan.BaseLineTrend {

	// 如果上一次查询的高度与当前高度相同，则不补充，适用于 0 点的时候
	if prev != nil && prev.Epoch == chain.CurrentEpoch() {
		return nil
	}

	// 每个高度计算一次
	if s.latestBaseLineTrend != nil && chain.CurrentEpoch()-s.latestBaseLineTrend.Epoch < 1 {
		return s.latestBaseLineTrend
	}

	// 通过查询链节点，补全最新的数据变化
	latest, err := s.adapter.Epoch(ctx, nil)
	if err != nil {
		return nil
	}

	if prev != nil {
		if chain.Epoch(latest.Epoch).CurrentDay()-prev.Epoch != 2880 {
			return nil
		}
	}

	adj, err := decimal.NewFromString(latest.NetQualityPower)
	if err != nil {
		return nil
	}

	raw, err := decimal.NewFromString(latest.NetPower)
	if err != nil {
		return nil
	}
	epoch := chain.Epoch(latest.Epoch)
	actor, err := s.adapter.Actor(ctx, chain.SmartAddress(builtin.RewardActorAddr.String()), &epoch)
	if err != nil {
		return nil
	}

	d, err := json.Marshal(actor.State)
	if err != nil {
		return nil
	}

	state, err := upgrader.UnmarshalerRewordState(d)
	if err != nil {
		return nil
	}

	item := &filscan.BaseLineTrend{
		TotalQualityAdjPower:  adj,
		TotalRawBytePower:     raw,
		BaseLinePower:         decimal.NewFromBigInt(state.ThisEpochBaselinePower.Int, 0),
		ChangeQualityAdjPower: decimal.Decimal{},
		Timestamp:             epoch.Time().Unix(),
		Epoch:                 epoch,
	}

	if prev != nil {
		item.ChangeQualityAdjPower = item.TotalQualityAdjPower.Sub(prev.TotalQualityAdjPower)
		epoch, err := s.minerPower.GetMaxEpochInPowerAbs(ctx)
		if err != nil {
			log.Error("get max epoch failed", epoch)
			return nil
		}
		if chain.Epoch(epoch).CurrentDay()-prev.Epoch != 2880 {
			goto RETURN
		}
		powers, err := s.minerPower.GetPowerAbs(ctx, chain.Epoch(epoch).CurrentDay().Int64(), epoch)
		if err != nil {
			log.Error("get power abs failed", epoch)
			return nil
		}
		totalToday, lossToday := decimal.Zero, decimal.Zero
		for i := range powers {
			totalToday = totalToday.Add(powers[i].PowerIncrease)
			lossToday = lossToday.Add(powers[i].PowerLoss)
		}

		item.PowerIncrease = totalToday
		item.PowerDecrease = lossToday
	}
RETURN:
	s.latestBaseLineTrend = item

	return item
}
