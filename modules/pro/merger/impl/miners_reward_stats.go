package mergerimpl

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type minersRewardStats struct {
	syncerGetter repository.SyncerGetter
	repo         prorepo.RewardRepo
}

func (m minersRewardStats) MinersRewardStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*merger.DayRewardStat, err error) {
	
	r, err := m.syncerGetter.GetSyncer(ctx, syncer.ChainSyncer)
	if err != nil {
		return
	}
	epoch = chain.Epoch(r.Epoch)
	
	date := dates.LteEnd
	for {
		dayStat := &merger.DayRewardStat{
			Day:   date,
			Stats: map[chain.SmartAddress]*merger.RewardStat{},
		}
		dayStat.Stats, err = m.minersRewardStats(ctx, miners, date)
		if err != nil {
			return
		}
		stats = append(stats, dayStat)
		date = date.SubDay()
		if date.Lt(dates.GteBegin) {
			break
		}
	}
	
	return
}

func (m minersRewardStats) minersRewardStats(ctx context.Context, miners []chain.SmartAddress, date chain.Date) (stats map[chain.SmartAddress]*merger.RewardStat, err error) {
	
	items, err := m.repo.GetMinerRewards(ctx, toAddrStrings(miners), date.SafeEpochs())
	if err != nil {
		return
	}
	
	stats = map[chain.SmartAddress]*merger.RewardStat{}
	res := map[string]*merger.RewardStat{}
	
	for _, v := range items {
		res[chain.SmartAddress(v.Miner).Address()] = &merger.RewardStat{
			Miner:        chain.SmartAddress(v.Miner),
			Blocks:       v.BlockCount,
			WinCounts:    v.WinCount,
			Rewards:      chain.AttoFil(v.Reward),
			TotalRewards: chain.AttoFil(v.AccReward),
		}
	}
	
	for _, v := range miners {
		if vv, ok := res[v.Address()]; ok {
			stats[v] = vv
		} else {
			stats[v] = &merger.RewardStat{
				Miner:        v,
				Blocks:       0,
				WinCounts:    0,
				Rewards:      chain.AttoFil{},
				TotalRewards: chain.AttoFil{},
			}
		}
	}
	
	return
}
