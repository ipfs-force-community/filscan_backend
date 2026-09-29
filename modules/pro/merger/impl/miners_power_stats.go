package mergerimpl

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
)

type MinersPowerStats interface {
	MinersPowerStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*merger.DayPowerStat, err error)
}

var _ MinersPowerStats = (*minersPowerStats)(nil)

type minersPowerStats struct {
	repo prorepo.MinerRepo
}

func (m minersPowerStats) MinersPowerStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*merger.DayPowerStat, err error) {
	
	epoch, err = m.repo.GetProInfoEpoch(ctx)
	if err != nil {
		return
	}
	
	//begin := dates.GteBegin.SafeEpochs().GteBegin
	//if epoch < begin {
	//	return
	//}
	
	date := dates.LteEnd
	for {
		var stat *merger.DayPowerStat
		stat, err = m.dayPowerStat(ctx, miners, epoch, date)
		if err != nil {
			return
		}
		stats = append(stats, stat)
		date = date.SubDay()
		if date.Lt(dates.GteBegin) {
			break
		}
	}
	
	return
}

func (m minersPowerStats) dayPowerStat(ctx context.Context, miners []chain.SmartAddress, latest chain.Epoch, date chain.Date) (stat *merger.DayPowerStat, err error) {
	
	epochs := date.SafeEpochs()
	
	//if date.IsToday(chain.TimeLoc) {
	//	if latest < epochs.LtEnd {
	//		epochs.LtEnd = latest
	//	}
	//}
	
	if latest < epochs.LtEnd {
		epochs.LtEnd = latest
		epochs.GteBegin = latest.CurrentDay()
	}
	
	//if latest < epochs.LtEnd {
	//	err = mix.Warnf("sync delay")
	//	return
	//}
	
	fmt.Printf("begin: %s end: %s", epochs.GteBegin, epochs.LtEnd)
	
	addrs := toAddrStrings(miners)
	// 准备 0 点的 INFO 计算差异值
	zeroItems, err := m.repo.GetMinerInfos(ctx, epochs.GteBegin.Int64(), addrs)
	if err != nil {
		return
	}
	zeroInfos := map[string]*propo.MinerInfo{}
	for _, v := range zeroItems {
		zeroInfos[v.Miner] = v
	}
	
	currentInfos, err := m.repo.GetMinerInfos(ctx, epochs.LtEnd.Int64(), addrs)
	if err != nil {
		return
	}
	
	// 用最新同步时间对齐，而不用链的最新高度
	funds, err := m.repo.GetMinerFunds(ctx, chain.NewLORCRange(epochs.GteBegin, epochs.LtEnd), addrs)
	if err != nil {
		return
	}
	
	stat = &merger.DayPowerStat{
		Day:   date,
		Stats: map[chain.SmartAddress]*merger.PowerStat{},
	}
	
	for _, v := range currentInfos {
		miner := chain.SmartAddress(v.Miner)
		item := &merger.PowerStat{
			Miner:                 miner,
			QualityAdjPower:       chain.Byte(v.QualityAdjPower),
			RawBytePower:          chain.Byte(v.RawBytePower),
			SectorSize:            chain.Byte(decimal.NewFromInt(v.SectorSize)),
			TotalSectors:          v.ActiveSectors,
			TotalSectorsZero:      0,
			TotalSectorsPowerZero: chain.Byte{},
			PledgeAmountZero:      chain.AttoFil{},
			PledgeAmountZeroPert:  chain.AttoFil{},
			PenaltyZero:           chain.AttoFil{},
			FaultSectors:          v.FaultSectors,
		}
		
		// VDC/CC（raw 口径：VdcPower + CcPower = RawBytePower）。
		//
		// 老口径 VDC = (QA-raw)/9 成立的前提是「QA 相对 raw 的超额只可能来自 verified deal 的
		// 10x」。NV29（Solstice / FIP-0118）之后这个前提没了：带 FULL_QA_POWER 的扇区同样贡献
		// 9*size 的超额，但 FIP-0118 之后没有 verified deal，这部分属于容量算力（CC）。
		// 聚合层只有 (QA, raw) 两个数，无法区分「老扇区的 VDW」与「新扇区的 FULL 标志」，
		// 因此 epoch >= UpgradeSolsticeHeight 时不再反推 VDC，超额一律按容量算力处理。
		// 逐 miner 的权威三桶（QA 口径）在 pro.miner_dcs / pro.miner_sectors，由 sector-task
		// 按 pkg/londobell.QASplit 的新口径写入。
		if epochs.LtEnd >= message_detail.UpgradeSolsticeHeight {
			item.VdcPower = chain.Byte{}
			item.CcPower = item.RawBytePower
		} else {
			item.VdcPower = chain.Byte(item.QualityAdjPower.Decimal().Sub(item.RawBytePower.Decimal()).Div(decimal.NewFromInt(9)))
			item.CcPower = chain.Byte(item.RawBytePower.Decimal().Sub(item.VdcPower.Decimal()))
		}
		
		if vv, ok := zeroInfos[v.Miner]; ok {
			item.TotalSectorsZero = v.LiveSectors - vv.LiveSectors
			item.TotalSectorsPowerZero = chain.Byte(decimal.NewFromInt(item.TotalSectorsZero).Mul(item.SectorSize.Decimal()))
			item.PledgeAmountZero = chain.AttoFil(v.Pledge.Sub(vv.Pledge))
			if item.TotalSectorsPowerZero.Decimal().GreaterThan(decimal.Zero) {
				item.PledgeAmountZeroPert = chain.AttoFil(item.PledgeAmountZero.Decimal().Div(item.TotalSectorsPowerZero.Decimal().Div(chain.PerT)))
			}
		}
		if vv, ok := funds[v.Miner]; ok {
			item.PenaltyZero = chain.AttoFil(vv.Penalty)
		}
		stat.Stats[miner] = item
	}
	return
}
