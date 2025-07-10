package calc_miner_acc_reward_task

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewCalcMinerAccRewardTask(repo repository.RewardTask) *CalcMinerAccRewardTask {
	return &CalcMinerAccRewardTask{repo: repo}
}

var _ syncer.Calculator = (*CalcMinerAccRewardTask)(nil)

type CalcMinerAccRewardTask struct {
	repo repository.RewardTask
}

func (m CalcMinerAccRewardTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (m CalcMinerAccRewardTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = m.repo.DeleteMinerRewardStats(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (m CalcMinerAccRewardTask) Name() string {
	return "calc-miner-acc-reward-task"
}

func (m CalcMinerAccRewardTask) Calc(ctx *syncer.Context) (err error) {
	if ctx.Empty() {
		return
	}

	rewardStats, err := m.prepareRewardStats(ctx)
	if err != nil {
		return
	}

	err = m.save(ctx.Context(), rewardStats)
	if err != nil {
		return
	}

	return
}

func (m CalcMinerAccRewardTask) save(ctx context.Context, rewardStats []*po.MinerRewardStat) (err error) {

	if len(rewardStats) > 0 {
		err = m.repo.SaveMinerRewardStats(ctx, rewardStats)
		if err != nil {
			return
		}
	}

	return
}

func (m CalcMinerAccRewardTask) prepareRewardStats(ctx *syncer.Context) (stats []*po.MinerRewardStat, err error) {

	stats24h, err := m.prepareAccReward(ctx.Context(), "24h", ctx.Epoch())
	if err != nil {
		return
	}
	stats7d, err := m.prepareAccReward(ctx.Context(), "7d", ctx.Epoch())
	if err != nil {
		return
	}
	stats30d, err := m.prepareAccReward(ctx.Context(), "30d", ctx.Epoch())
	if err != nil {
		return
	}
	stats1y, err := m.prepareAccReward(ctx.Context(), "1y", ctx.Epoch())
	if err != nil {
		return
	}

	stats = append(stats, stats24h, stats7d, stats30d, stats1y)

	return
}

func (m CalcMinerAccRewardTask) prepareAccReward(ctx context.Context, interval string, epoch chain.Epoch) (stat *po.MinerRewardStat, err error) {

	var pre chain.Epoch
	switch interval {
	case "24h":
		pre = epoch - 2880
	case "7d":
		pre = epoch - 2880*7
	case "30d":
		pre = epoch - 2880*30
	case "1y":
		pre = epoch - 2880*365
	default:
		err = fmt.Errorf("unsuport interval: %s", interval)
		return
	}

	reward, err := m.repo.SumRewards(ctx, chain.NewLCRCRange(pre, epoch))
	if err != nil {
		return
	}

	power, err := m.repo.GetNetQualityAdjPower(ctx, epoch)
	if err != nil {
		return
	}

	stat = &po.MinerRewardStat{
		Epoch:         epoch.Int64(),
		Interval:      interval,
		AccReward:     reward,
		AccRewardPerT: decimal.Decimal{},
	}

	if power.GreaterThan(decimal.Zero) {
		stat.AccRewardPerT = reward.Div(power.Div(chain.PerT))
	}

	return
}
