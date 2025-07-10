package mergerimpl

import (
	"context"
	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type minersFundStats struct {
	repo prorepo.MinerRepo
}

func (m minersFundStats) MinersFundStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*merger.DayFundStat, err error) {
	
	epoch, err = m.repo.GetProInfoEpoch(ctx)
	if err != nil {
		return
	}
	addrs := toAddrStrings(miners)
	
	date := dates.LteEnd
	for {
		dayStat := &merger.DayFundStat{
			Day:   date,
			Stats: map[chain.SmartAddress]*merger.FundStat{},
		}
		
		var minerStats []*merger.FundStat
		minerStats, err = m.minerFundStat(ctx, epoch, addrs, date)
		if err != nil {
			return
		}
		
		mapping := map[string]*merger.FundStat{}
		for _, v := range minerStats {
			mapping[v.Miner.Address()] = v
		}
		
		for _, v := range miners {
			dayStat.Stats[v] = &merger.FundStat{
				Miner:                 v,
				TotalSectorsZero:      0,
				TotalSectorsPowerZero: chain.Byte{},
				TotalGas:              chain.AttoFil{},
				SealGas:               chain.AttoFil{},
				SealGasPerT:           chain.AttoFil{},
				PublishDealGas:        chain.AttoFil{},
				WdPostGas:             chain.AttoFil{},
				WdPostGasPerT:         chain.AttoFil{},
			}
			if vv, ok := mapping[v.Address()]; ok {
				dayStat.Stats[v].TotalSectorsZero = vv.TotalSectorsZero
				dayStat.Stats[v].TotalSectorsPowerZero = vv.TotalSectorsPowerZero
				dayStat.Stats[v].TotalGas = vv.TotalGas
				dayStat.Stats[v].SealGas = vv.SealGas
				dayStat.Stats[v].SealGasPerT = vv.SealGasPerT
				dayStat.Stats[v].PublishDealGas = vv.PublishDealGas
				dayStat.Stats[v].WdPostGas = vv.WdPostGas
				dayStat.Stats[v].WdPostGasPerT = vv.WdPostGasPerT
			}
		}
		
		stats = append(stats, dayStat)
		
		date = date.SubDay()
		if date.Lt(dates.GteBegin) {
			break
		}
	}
	
	return
}

type MinerChangePower struct {
	ChangeSector int64
	ChangePower  decimal.Decimal
}

func (m minersFundStats) minerFundStat(ctx context.Context, latest chain.Epoch, miners []string, date chain.Date) (stats []*merger.FundStat, err error) {
	
	epochs := date.SafeEpochs()
	
	if latest < epochs.LtEnd {
		epochs.LtEnd = latest
		epochs.GteBegin = latest.CurrentDay()
	}
	
	items, err := m.repo.GetMinerFunds(ctx, chain.NewLORCRange(epochs.GteBegin, epochs.LtEnd), miners)
	if err != nil {
		return
	}
	
	// 准备 0 点的 INFO 计算差异值
	zeroItems, err := m.repo.GetMinerInfos(ctx, epochs.GteBegin.Int64(), miners)
	if err != nil {
		return
	}
	zeroInfos := map[string]*propo.MinerInfo{}
	for _, v := range zeroItems {
		zeroInfos[v.Miner] = v
	}
	
	currentInfos, err := m.repo.GetMinerInfos(ctx, epochs.LtEnd.Int64(), miners)
	if err != nil {
		return
	}
	
	changePower := map[string]*MinerChangePower{}
	for _, v := range currentInfos {
		if vv, ok := zeroInfos[v.Miner]; ok {
			changePower[v.Miner] = &MinerChangePower{
				ChangeSector: v.LiveSectors - vv.LiveSectors,
				ChangePower:  decimal.NewFromInt(v.LiveSectors - vv.LiveSectors).Mul(decimal.NewFromInt(v.SectorSize)),
			}
		}
	}
	
	for _, v := range items {
		item := &merger.FundStat{
			Miner:                 chain.SmartAddress(v.Miner),
			TotalSectorsZero:      0,
			TotalSectorsPowerZero: chain.Byte{},
			TotalGas:              chain.AttoFil(v.TotalGas),
			SealGas:               chain.AttoFil(v.SealGas),
			SealGasPerT:           chain.AttoFil{},
			PublishDealGas:        chain.AttoFil(v.DealGas),
			WdPostGas:             chain.AttoFil(v.WdPostGas),
			WdPostGasPerT:         chain.AttoFil{},
		}
		
		if vv, ok := changePower[v.Miner]; ok {
			item.TotalSectorsZero = vv.ChangeSector
			item.TotalSectorsPowerZero = chain.Byte(vv.ChangePower)
			if vv.ChangePower.GreaterThan(decimal.Zero) {
				item.WdPostGasPerT = chain.AttoFil(v.WdPostGas.Div(vv.ChangePower.Div(chain.PerT)))
				item.SealGasPerT = chain.AttoFil(v.SealGas.Div(vv.ChangePower.Div(chain.PerT)))
			}
		}
		
		stats = append(stats, item)
	}
	
	return
}
