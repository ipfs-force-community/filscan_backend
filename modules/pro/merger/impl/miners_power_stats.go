package mergerimpl

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
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

		// 满倍率 / 可升级 两档（raw 口径：VdcPower + CcPower = RawBytePower）。
		//
		// 口径 chain.QualityTierSplit（NV29/FIP-0118 方案 A），两时代同式、不再按 epoch 分叉：
		//   VdcPower（语义＝满倍率算力）= (QA-raw)/9  ← 处于 10× 档的等效原始字节
		//   CcPower （语义＝可升级算力）= raw - 满倍率  ← 未达满倍率的等效原始字节
		// NV29 前前者恰等于旧 VDC、后者恰等于旧 CC ⇒ 历史不重算、曲线连续。
		// merger 内部字段名 VdcPower/CcPower 不改（波及面另评），此处只改语义并注明。
		// 逐 miner 的权威三桶（QA 口径）在 pro.miner_dcs / pro.miner_sectors，由 sector-task
		// 按 pkg/londobell.QASplit 写入——与本节口径不同，勿混用。
		full, pending := chain.QualityTierSplit(item.QualityAdjPower.Decimal(), item.RawBytePower.Decimal())
		item.VdcPower = chain.Byte(full)
		item.CcPower = chain.Byte(pending)

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
