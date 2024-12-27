package mergerimpl

import (
	"context"
	"github.com/gozelle/async/parallel"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type minersBalanceStats struct {
	*heightGetter
	adapter londobell.Adapter
}

func (m minersBalanceStats) MinersBalanceStats(ctx context.Context, miners []chain.SmartAddress, date chain.Date) (epoch chain.Epoch, stats map[chain.SmartAddress]*merger.BalanceStat, err error) {
	
	adapterEpoch, err := m.AdapterHeight(ctx)
	if err != nil {
		return
	}
	epochs := date.SafeEpochs()
	epochs.LtEnd = adapterEpoch
	epoch = adapterEpoch
	
	m1, err := m.adapter.BalanceAtMarket(ctx, miners, epochs.LtEnd)
	if err != nil {
		return
	}
	m1m := map[string]decimal.Decimal{}
	for _, v := range m1 {
		m1m[v.Actor] = v.EscrowBalance.Sub(v.LockedBalance)
	}
	m2, err := m.adapter.BalanceAtMarket(ctx, miners, epochs.GteBegin)
	if err != nil {
		return
	}
	m2m := map[string]decimal.Decimal{}
	for _, v := range m2 {
		m2m[v.Actor] = v.EscrowBalance.Sub(v.LockedBalance)
	}
	
	var runners []parallel.Runner[*merger.BalanceStat]
	
	for _, v := range miners {
		
		miner := v
		runners = append(runners, func(ctx context.Context) (*merger.BalanceStat, error) {
			return m.minerBalanceStats(ctx, miner, epochs)
		})
		
	}
	
	stats = map[chain.SmartAddress]*merger.BalanceStat{}
	ch := parallel.Run[*merger.BalanceStat](ctx, 20, runners)
	err = parallel.Wait[*merger.BalanceStat](ch, func(v *merger.BalanceStat) error {
		stats[v.Addr] = v
		stats[v.Addr].Market = chain.AttoFil(m1m[v.Addr.Address()])
		stats[v.Addr].MarketZero = chain.AttoFil(m1m[v.Addr.Address()].Sub(m2m[v.Addr.Address()]))
		return nil
	})
	if err != nil {
		return
	}
	
	return
}

func (m minersBalanceStats) minerBalanceStats(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (stat *merger.BalanceStat, err error) {
	
	end, err := m.requestBalance(ctx, miner, epochs.LtEnd)
	if err != nil {
		return
	}
	
	start, err := m.requestBalance(ctx, miner, epochs.GteBegin)
	if err != nil {
		return
	}
	
	stat = &merger.BalanceStat{
		Addr:            miner,
		Miner:           chain.AttoFil(end.Miner),
		MinerZero:       chain.AttoFil(end.Miner.Sub(start.Miner)),
		Owner:           chain.AttoFil(end.Owner),
		OwnerZero:       chain.AttoFil(end.Owner.Sub(start.Owner)),
		Worker:          chain.AttoFil(end.Worker),
		WorkerZero:      chain.AttoFil(end.Worker.Sub(start.Worker)),
		C0:              chain.AttoFil(end.C0),
		C0Zero:          chain.AttoFil(end.C0.Sub(start.C0)),
		C1:              chain.AttoFil(end.C1),
		C1Zero:          chain.AttoFil(end.C1.Sub(start.C1)),
		C2:              chain.AttoFil(end.C2),
		C2Zero:          chain.AttoFil(end.C2.Sub(start.C2)),
		Beneficiary:     chain.AttoFil(end.Beneficiary),
		BeneficiaryZero: chain.AttoFil(end.Beneficiary.Sub(start.Beneficiary)),
		Market:          chain.AttoFil(end.Market),
		MarketZero:      chain.AttoFil(end.Market.Sub(start.Market)),
	}
	
	return
}

type minerBalance struct {
	Miner       decimal.Decimal
	Owner       decimal.Decimal
	Worker      decimal.Decimal
	C0          decimal.Decimal
	C1          decimal.Decimal
	C2          decimal.Decimal
	Beneficiary decimal.Decimal
	Market      decimal.Decimal
}

func (m minersBalanceStats) requestBalance(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (stat *minerBalance, err error) {
	
	info, err := m.adapter.Miner(ctx, miner, &epoch)
	if err != nil {
		return
	}
	
	g := parallel.NewGroup()
	
	var (
		ob  decimal.Decimal
		wb  decimal.Decimal
		c0b decimal.Decimal
		c1b decimal.Decimal
		c2b decimal.Decimal
		bb  decimal.Decimal
		mb  decimal.Decimal
	)
	
	g.Go(func() error {
		if info.Owner != "" {
			actor, e := m.adapter.Actor(ctx, chain.SmartAddress(info.Owner), &epoch)
			if e != nil {
				return e
			}
			ob = actor.Balance
		}
		return nil
	})
	
	g.Go(func() error {
		if info.Owner != "" {
			actor, e := m.adapter.Actor(ctx, chain.SmartAddress(info.Worker), &epoch)
			if e != nil {
				return e
			}
			wb = actor.Balance
		}
		return nil
	})
	
	g.Go(func() error {
		if info.Owner != "" {
			actor, e := m.adapter.Actor(ctx, chain.SmartAddress(info.Beneficiary), &epoch)
			if e != nil {
				return e
			}
			bb = actor.Balance
		}
		return nil
	})
	
	g.Go(func() error {
		if len(info.Controllers) > 0 {
			actor, e := m.adapter.Actor(ctx, chain.SmartAddress(info.Controllers[0]), &epoch)
			if e != nil {
				return e
			}
			c0b = actor.Balance
		}
		return nil
	})
	
	g.Go(func() error {
		if len(info.Controllers) > 1 {
			actor, e := m.adapter.Actor(ctx, chain.SmartAddress(info.Controllers[1]), &epoch)
			if e != nil {
				return e
			}
			c1b = actor.Balance
		}
		return nil
	})
	
	g.Go(func() error {
		if len(info.Controllers) > 2 {
			actor, e := m.adapter.Actor(ctx, chain.SmartAddress(info.Controllers[2]), &epoch)
			if e != nil {
				return e
			}
			c2b = actor.Balance
		}
		return nil
	})
	
	err = g.Wait()
	if err != nil {
		return
	}
	stat = &minerBalance{
		Miner:       info.Balance,
		Owner:       ob,
		Worker:      wb,
		C0:          c0b,
		C1:          c1b,
		C2:          c2b,
		Beneficiary: bb,
		Market:      mb,
	}
	
	return
}
