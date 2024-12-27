package mergerimpl

import (
	"context"
	"github.com/gozelle/async/collection"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

type minersSectorStats struct {
	db   *gorm.DB
	repo prorepo.MinerRepo
}

func (m minersSectorStats) MinersSectorStats(ctx context.Context, miners []chain.SmartAddress) (epoch chain.Epoch, stats *merger.SectorStat, err error) {
	
	err = m.db.Raw(`select max(epoch) from pro.miner_sectors where epoch <= (select epoch from chain.sync_syncers where name = ?)`, syncer.SectorSyncer).Scan(&epoch).Error
	if err != nil {
		return
	}
	
	sectors, err := m.repo.GetMinersSectors(ctx, epoch.Int64(), toAddrStrings(miners))
	
	stats = &merger.SectorStat{}
	stats.Months, stats.Days = m.cacExpirations(sectors)
	
	return
}

func (m minersSectorStats) cacExpirations(items []*propo.MinerSector) (months []*merger.MonthSectorStat, days []*merger.DaySectorStat) {
	
	monthsMap := map[string]map[string]*merger.MinerSectorStat{}
	daysMap := map[string]map[string]*merger.MinerSectorStat{}
	
	for _, v := range items {
		hour := chain.Epoch(v.HourEpoch)
		month := hour.Time().Format("2006-01")
		day := hour.Date()
		m.calcSectorMap(&monthsMap, month, v)
		m.calcSectorMap(&daysMap, day, v)
	}
	
	for k, v := range monthsMap {
		monthStat := &merger.MonthSectorStat{
			Month: k,
		}
		for _, vv := range v {
			monthStat.Miners = append(monthStat.Miners, vv)
			monthStat.Sectors += vv.Sectors
			monthStat.Pledge = chain.AttoFil(monthStat.Pledge.Decimal().Add(vv.Pledge.Decimal()))
			monthStat.Power = chain.Byte(monthStat.Power.Decimal().Add(vv.Power.Decimal()))
			monthStat.VDC = chain.Byte(monthStat.VDC.Decimal().Add(vv.VDC.Decimal()))
			monthStat.DC = chain.Byte(monthStat.DC.Decimal().Add(vv.DC.Decimal()))
			monthStat.CC = chain.Byte(monthStat.CC.Decimal().Add(vv.CC.Decimal()))
		}
		months = append(months, monthStat)
	}
	collection.Sort(months, func(a, b *merger.MonthSectorStat) bool {
		return a.Month < b.Month
	})
	for k, v := range daysMap {
		dayStat := &merger.DaySectorStat{
			Day: k,
		}
		for _, vv := range v {
			dayStat.Miners = append(dayStat.Miners, vv)
			dayStat.Sectors += vv.Sectors
			dayStat.Pledge = chain.AttoFil(dayStat.Pledge.Decimal().Add(vv.Pledge.Decimal()))
			dayStat.Power = chain.Byte(dayStat.Power.Decimal().Add(vv.Power.Decimal()))
			dayStat.VDC = chain.Byte(dayStat.VDC.Decimal().Add(vv.VDC.Decimal()))
			dayStat.DC = chain.Byte(dayStat.DC.Decimal().Add(vv.DC.Decimal()))
			dayStat.CC = chain.Byte(dayStat.CC.Decimal().Add(vv.CC.Decimal()))
		}
		days = append(days, dayStat)
	}
	collection.Sort(days, func(a, b *merger.DaySectorStat) bool {
		return a.Day < b.Day
	})
	return
}

func (m minersSectorStats) calcSectorMap(mapping *map[string]map[string]*merger.MinerSectorStat, key string, item *propo.MinerSector) {
	if _, ok := (*mapping)[key]; !ok {
		(*mapping)[key] = map[string]*merger.MinerSectorStat{}
	}
	if _, ok := (*mapping)[key][item.Miner]; !ok {
		(*mapping)[key][item.Miner] = &merger.MinerSectorStat{Miner: chain.SmartAddress(item.Miner)}
	}
	(*mapping)[key][item.Miner].Sectors += item.Sectors
	(*mapping)[key][item.Miner].Power = chain.Byte((*mapping)[key][item.Miner].Power.Decimal().Add(item.Power))
	(*mapping)[key][item.Miner].Pledge = chain.AttoFil((*mapping)[key][item.Miner].Pledge.Decimal().Add(item.Pledge))
	
	(*mapping)[key][item.Miner].VDC = chain.Byte(item.Vdc)
	(*mapping)[key][item.Miner].DC = chain.Byte(item.Dc)
	(*mapping)[key][item.Miner].CC = chain.Byte(item.Cc)
}
