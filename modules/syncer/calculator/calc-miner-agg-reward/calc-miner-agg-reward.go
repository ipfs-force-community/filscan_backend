package calc_miner_agg_reward

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewCalcMinerAggReward(repo repository.RewardTask) *CalcMinerAggReward {
	return &CalcMinerAggReward{repo: repo}
}

var _ syncer.Calculator = (*CalcMinerAggReward)(nil)

type CalcMinerAggReward struct {
	repo repository.RewardTask
}

func (c CalcMinerAggReward) Name() string {
	return "calc-miner-agg-reward"
}

func (c CalcMinerAggReward) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	return
}

func (c CalcMinerAggReward) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c CalcMinerAggReward) Calc(ctx *syncer.Context) (err error) {
	
	if !ctx.LastCalc() {
		return
	}
	ctx.Debugf("开始更新 MinerAggReward")
	miners, err := c.repo.GetRewardMiners(ctx.Context(), ctx.Epochs())
	if err != nil {
		return
	}
	
	aggRewards, err := c.repo.SumMinersTotalRewards(ctx.Context(), miners)
	if err != nil {
		return
	}
	
	err = c.repo.SaveMinerAggReward(ctx.Context(), aggRewards)
	if err != nil {
		return
	}
	
	return
}
