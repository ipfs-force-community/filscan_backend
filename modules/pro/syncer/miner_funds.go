package prosyncer

import (
	"context"
	"fmt"

	"github.com/gozelle/async/parallel"
	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type minerFunds struct {
}

func (m minerFunds) prepareMinerFees(ctx *syncer.Context, epochs chain.LCRORange, miners []*londobell.MinerInfo) (fees []*propo.MinerFund, err error) {

	rewards, err := m.prepareMinerRewards(ctx, epochs)
	if err != nil {
		return
	}
	winCounts, err := m.prepareMinerWinCounts(ctx, epochs)
	if err != nil {
		return
	}
	preAggs, err := m.prepareMinerPreAgg(ctx, epochs)
	if err != nil {
		return
	}
	proAggs, err := m.prepareMinerProAgg(ctx, epochs)
	if err != nil {
		return
	}

	var runners []parallel.Runner[*propo.MinerFund]
	for _, v := range miners {
		miner := v
		runners = append(runners, func(_ context.Context) (*propo.MinerFund, error) {
			return m.prepareMinerFee(ctx.Context(), ctx.MinerAgg(), rewards, winCounts, preAggs, proAggs, epochs, miner.Miner) //todo 这里数据可能有问题
		})
	}

	ch := parallel.Run[*propo.MinerFund](ctx.Context(), 5, runners)
	err = parallel.Wait[*propo.MinerFund](ch, func(v *propo.MinerFund) error {
		fees = append(fees, v)
		return nil
	})
	if err != nil {
		return
	}

	return
}

type MinerReward struct {
	Reward     decimal.Decimal
	BlockCount int64
}

func (m minerFunds) prepareMinerRewards(ctx *syncer.Context, epochs chain.LCRORange) (rewards map[string]*MinerReward, err error) {

	items, err := ctx.Agg().MinersBlockReward(ctx.Context(), epochs.GteBegin, epochs.LtEnd)
	if err != nil {
		return
	}
	rewards = map[string]*MinerReward{}
	for _, v := range items {
		miner := chain.SmartAddress(v.Id.Miner).Address()
		if _, ok := rewards[miner]; !ok {
			rewards[miner] = &MinerReward{}
		}
		rewards[miner].Reward = rewards[miner].Reward.Add(v.TotalBlockReward)
		rewards[miner].BlockCount += v.BlockCount
	}

	return
}

func (m minerFunds) prepareMinerWinCounts(ctx *syncer.Context, epochs chain.LCRORange) (winCounts map[string]int64, err error) {

	items, err := ctx.Agg().WinCount(ctx.Context(), epochs.GteBegin, epochs.LtEnd)
	if err != nil {
		return
	}
	winCounts = map[string]int64{}
	for _, v := range items {
		miner := chain.SmartAddress(v.Id).Address()
		if _, ok := winCounts[miner]; !ok {
			winCounts[miner] = 0
		}
		winCounts[miner] += v.TotalWinCount
	}

	return
}

func (m minerFunds) prepareMinerPreAgg(ctx *syncer.Context, epochs chain.LCRORange) (preAggs map[string]decimal.Decimal, err error) {

	items, err := ctx.Agg().AggPreNetFee(ctx.Context(), epochs.GteBegin, epochs.LtEnd)
	if err != nil {
		return
	}
	preAggs = map[string]decimal.Decimal{}
	for _, v := range items {
		miner := chain.SmartAddress(v.Miner).Address()
		if _, ok := preAggs[miner]; !ok {
			preAggs[miner] = decimal.Decimal{}
		}
		preAggs[miner] = preAggs[miner].Add(v.AggFee)
	}

	return
}

func (m minerFunds) prepareMinerProAgg(ctx *syncer.Context, epochs chain.LCRORange) (proAggs map[string]decimal.Decimal, err error) {

	items, err := ctx.Agg().AggProNetFee(ctx.Context(), epochs.GteBegin, epochs.LtEnd)
	if err != nil {
		return
	}
	proAggs = map[string]decimal.Decimal{}
	for _, v := range items {
		miner := v.Miner.Address()
		if _, ok := proAggs[miner]; !ok {
			proAggs[miner] = decimal.Decimal{}
		}
		proAggs[miner] = proAggs[miner].Add(v.AggFee)
	}

	return
}

func (m minerFunds) prepareMinerFee(ctx context.Context, minerAgg londobell.MinerAgg, rewards map[string]*MinerReward,
	winCounts map[string]int64, preAggs, proAggs map[string]decimal.Decimal, epochs chain.LCRORange, miner chain.SmartAddress) (fund *propo.MinerFund, err error) {

	fund = &propo.MinerFund{
		Epoch:      epochs.LtEnd.Int64(),
		Miner:      miner.Address(),
		Income:     decimal.Decimal{},
		Outlay:     decimal.Decimal{},
		TotalGas:   decimal.Decimal{},
		SealGas:    decimal.Decimal{},
		DealGas:    decimal.Decimal{},
		WdPostGas:  decimal.Decimal{},
		Penalty:    decimal.Decimal{},
		Reward:     decimal.Decimal{},
		BlockCount: 0,
		WinCount:   0,
		OtherGas:   decimal.Decimal{},
		PreAgg:     decimal.Decimal{},
		ProAgg:     decimal.Decimal{},
	}

	if v, ok := rewards[miner.Address()]; ok {
		fund.Reward = v.Reward
		fund.BlockCount = v.BlockCount
	}

	if v, ok := winCounts[miner.Address()]; ok {
		fund.WinCount = v
	}

	if v, ok := preAggs[miner.Address()]; ok {
		fund.PreAgg = v
	}

	if v, ok := proAggs[miner.Address()]; ok {
		fund.ProAgg = v
	}

	commonCosts, err := minerAgg.PeriodGasCost(ctx, miner, epochs)
	if err != nil {
		err = fmt.Errorf("prepare miner: %s gas cost error: %w", miner, err)
		return
	}
	fund.SealGas, fund.WdPostGas, fund.OtherGas = m.extractMinerGases(commonCosts)

	dealCosts, err := minerAgg.PeriodGasCostForPublishDeals(ctx, miner, epochs)
	if err != nil {
		err = fmt.Errorf("prepare miner: %s deal cost error: %w", miner, err)
		return
	}
	fund.DealGas = m.extractPublishStorageDealGas(dealCosts)

	penalty, err := minerAgg.PeriodPunishments(ctx, miner, epochs)
	if err != nil {
		err = fmt.Errorf("prepare miner: %s publishments error: %w", miner, err)
		return
	}
	if penalty != nil {
		fund.Penalty = penalty.Punishments
	}

	for _, v := range append(commonCosts, dealCosts...) {
		fund.TotalGas = fund.TotalGas.Add(v.GasCosts)
	}
	bill, err := minerAgg.PeriodBill(ctx, miner, epochs)
	if err != nil {
		return
	}
	if bill != nil {
		fund.Income = bill.Income
		fund.Outlay = bill.Pay
	}

	return
}

func (m minerFunds) extractMinerGases(items []*londobell.PeriodGasCostResp) (sealGas, wdPostGas, otherGas decimal.Decimal) {
	for _, v := range items {
		switch v.Method {
		case "SubmitWindowedPoSt":
			wdPostGas = wdPostGas.Add(v.GasCosts)
		case "PreCommitSector",
			"PreCommitSectorBatch",
			"PreCommitSectorBatch2",
			"ProveCommitAggregate",
			"ProveCommitSector":
			sealGas = sealGas.Add(v.GasCosts)
		default:
			otherGas = otherGas.Add(v.GasCosts)
		}
	}
	return
}

func (m minerFunds) extractPublishStorageDealGas(items []*londobell.PeriodGasCostResp) (publishStorageDealGas decimal.Decimal) {
	for _, v := range items {
		publishStorageDealGas = publishStorageDealGas.Add(v.GasCosts)
	}
	return
}
