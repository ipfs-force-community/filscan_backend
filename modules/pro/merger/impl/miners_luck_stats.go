package mergerimpl

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type minersLuckStats struct {
	epochGetter repository.SyncEpochGetter
	repo        prorepo.LuckRepo
}

func (m minersLuckStats) MinersLuckStats(ctx context.Context, miners []chain.SmartAddress) (epoch chain.Epoch, stats map[chain.SmartAddress]*merger.LuckStats, err error) {
	
	r, err := m.epochGetter.MinerEpoch(ctx)
	if err != nil {
		return
	}
	
	if r == nil {
		err = fmt.Errorf("stat miners luck: epoch is nil")
		return
	}
	
	epoch = *r
	
	lucks, err := m.repo.QueryMinerLucks(ctx, toAddrStrings(miners), epoch.Int64())
	if err != nil {
		return
	}
	
	stats = map[chain.SmartAddress]*merger.LuckStats{}
	for _, v := range miners {
		if vv, ok := lucks[v.Address()]; ok {
			stats[v] = &merger.LuckStats{
				Luck24h: vv.Luck24h,
				Luck7d:  vv.Luck7d,
				Luck30d: vv.Luck30d,
			}
		} else {
			stats[v] = &merger.LuckStats{
				Luck24h: decimal.Zero,
				Luck7d:  decimal.Zero,
				Luck30d: decimal.Zero,
			}
		}
	}
	
	return
}
