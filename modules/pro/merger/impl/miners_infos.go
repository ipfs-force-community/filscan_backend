package mergerimpl

import (
	"context"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func toAddrStrings(addresses []chain.SmartAddress) []string {
	var items []string
	for _, v := range addresses {
		items = append(items, v.Address())
	}
	return items
}

type MinerInfos interface {
	MinersInfos(ctx context.Context, miners []chain.SmartAddress, date chain.Date) (epoch chain.Epoch, summary merger.MinersSummary, infos map[chain.SmartAddress]*merger.MinerInfo, err error)
}

var _ MinerInfos = (*minerInfos)(nil)

type minerInfos struct {
	syncer   prorepo.SyncerRepo
	repo     prorepo.MinerRepo
	agg      londobell.Agg
	minerAgg londobell.MinerAgg
	adapter  londobell.Adapter
}

func (m minerInfos) MinersInfos(ctx context.Context, miners []chain.SmartAddress, date chain.Date) (epoch chain.Epoch, summary merger.MinersSummary, infos map[chain.SmartAddress]*merger.MinerInfo, err error) {

	epoch, err = m.repo.GetProInfoEpoch(ctx)
	if err != nil {
		return
	}
	begin := date.SafeEpochs().GteBegin

	if epoch < begin {
		begin = epoch.CurrentDay()
	}

	addrs := toAddrStrings(miners)

	// 准备 0 点的 INFO 计算差异值
	zeroItems, err := m.repo.GetMinerInfos(ctx, begin.Int64(), addrs)
	if err != nil {
		return
	}
	zeroInfos := map[string]*propo.MinerInfo{}
	for _, v := range zeroItems {
		zeroInfos[v.Miner] = v
	}

	currentInfos, err := m.repo.GetMinerInfos(ctx, epoch.Int64(), addrs)
	if err != nil {
		return
	}

	// 用最新同步时间对齐，而不用链的最新高度
	funds, err := m.repo.GetMinerFunds(ctx, chain.NewLORCRange(begin, epoch), addrs)
	if err != nil {
		return
	}

	zeroBalances, err := m.repo.GetMinerBalances(ctx, begin.Int64(), addrs)
	if err != nil {
		return
	}

	currentBalances, err := m.repo.GetMinerBalances(ctx, epoch.Int64(), addrs)
	if err != nil {
		return
	}

	infos = map[chain.SmartAddress]*merger.MinerInfo{}
	for _, v := range currentInfos {
		infos[chain.SmartAddress(v.Miner)] = m.prepareMinerInfo(v, zeroInfos, funds, zeroBalances, currentBalances)
	}

	summary = m.prepareSummary(infos)

	return
}

func (m minerInfos) prepareMinerInfo(item *propo.MinerInfo, zeroInfos map[string]*propo.MinerInfo, funds map[string]*propo.MinerFund,
	zeroBalances, currentBalances map[string]*propo.MinerBalance) (info *merger.MinerInfo) {

	info = &merger.MinerInfo{
		QualityAdjPower: chain.Byte(item.QualityAdjPower),
		RawBytePower:    chain.Byte(item.RawBytePower),
		Reward:          chain.AttoFil{},
		RewardZero:      chain.AttoFil{},
		Outlay:          chain.AttoFil{},
		Gas:             chain.AttoFil{},
		PledgeAmount:    chain.AttoFil(item.Pledge),
		PledgeZero:      chain.AttoFil{},
		Balance:         chain.AttoFil{},
		BalanceZero:     chain.AttoFil{},
	}

	if v, ok := zeroInfos[item.Miner]; ok {
		info.PledgeZero = chain.AttoFil(info.PledgeAmount.Decimal().Sub(v.Pledge))
		info.QualityAdjPowerZero = chain.Byte(info.QualityAdjPower.Decimal().Sub(v.QualityAdjPower))
	} else {
		info.PledgeZero = info.PledgeAmount
		info.QualityAdjPowerZero = info.QualityAdjPower
	}

	if v, ok := funds[item.Miner]; ok {
		info.RewardZero = chain.AttoFil(v.Reward)             // 此处是当日奖励变化
		info.Outlay = chain.AttoFil(v.Outlay.Add(v.TotalGas)) // Miner 总支出等于 Miner支出 + 关联手续费
		info.Gas = chain.AttoFil(v.TotalGas)
	}

	if v, ok := currentBalances[item.Miner]; ok {
		info.Balance = chain.AttoFil(v.Balance) // 此处是到 0 点的累计奖励数
	}

	if v, ok := zeroBalances[item.Miner]; ok {
		info.BalanceZero = chain.AttoFil(info.Balance.Decimal().Sub(v.Balance))
	} else {
		info.BalanceZero = info.Balance
	}

	return
}

func (m minerInfos) prepareSummary(infos map[chain.SmartAddress]*merger.MinerInfo) (summary merger.MinersSummary) {

	for _, v := range infos {
		summary.TotalBalance = chain.AttoFil(summary.TotalBalance.Decimal().Add(v.Balance.Decimal()))
		summary.TotalQualityAdjPower = chain.Byte(summary.TotalQualityAdjPower.Decimal().Add(v.QualityAdjPower.Decimal()))
		summary.TotalQualityAdjPowerZero = chain.Byte(summary.TotalQualityAdjPowerZero.Decimal().Add(v.QualityAdjPowerZero.Decimal()))
		//summary.TotalReward = chain.AttoFil(summary.TotalReward.Decimal().Add(v.Reward.Decimal()))
		summary.TotalRewardZero = chain.AttoFil(summary.TotalRewardZero.Decimal().Add(v.RewardZero.Decimal()))
		summary.TotalOutcome = chain.AttoFil(summary.TotalOutcome.Decimal().Add(v.Outlay.Decimal()))
		summary.TotalGas = chain.AttoFil(summary.TotalGas.Decimal().Add(v.Gas.Decimal()))
		summary.TotalPledge = chain.AttoFil(summary.TotalPledge.Decimal().Add(v.PledgeAmount.Decimal()))
		summary.TotalPledgeZero = chain.AttoFil(summary.TotalPledgeZero.Decimal().Add(v.PledgeZero.Decimal()))
		//summary.TotalBalance = chain.AttoFil(summary.TotalBalance.Decimal().Add(v.Balance.Decimal()))
		summary.TotalBalanceZero = chain.AttoFil(summary.TotalBalanceZero.Decimal().Add(v.BalanceZero.Decimal()))
	}

	return
}
