package prosyncer

import (
	"context"
	"fmt"
	"github.com/gozelle/async/parallel"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type minerBalances struct {
}

func (m minerBalances) prepareMinerBalances(ctx *syncer.Context, epoch chain.Epoch, infos []*propo.MinerInfo) (balances []*propo.MinerBalance, err error) {
	
	var addrs []chain.SmartAddress
	var runners []parallel.Runner[[]*propo.MinerBalance]
	for _, v := range infos {
		addrs = append(addrs, chain.SmartAddress(v.Miner))
		info := v
		runners = append(runners, func(_ context.Context) ([]*propo.MinerBalance, error) {
			return m.queryMinerBalances(ctx, info, epoch)
		})
	}
	
	ch := parallel.Run[[]*propo.MinerBalance](ctx.Context(), 3, runners)
	err = parallel.Wait[[]*propo.MinerBalance](ch, func(v []*propo.MinerBalance) error {
		balances = append(balances, v...)
		return nil
	})
	if err != nil {
		return
	}
	
	markets, err := ctx.Adapter().BalanceAtMarket(ctx.Context(), addrs, epoch)
	if err != nil {
		err = fmt.Errorf("prepare market balance error: %w", err)
		return
	}
	
	for _, v := range markets {
		balances = append(balances, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Miner:   v.Actor,
			Type:    "market",
			Address: "",
			Balance: v.EscrowBalance,
		})
	}
	
	return
}

func (m minerBalances) queryMinerBalances(ctx *syncer.Context, info *propo.MinerInfo, epoch chain.Epoch) (items []*propo.MinerBalance, err error) {
	
	defer func() {
		for _, v := range items {
			v.Miner = info.Miner
		}
	}()
	
	{
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Miner), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s miner: %s balance error: %w", info.Miner, info.Miner, err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "miner",
			Address: info.Miner,
			Balance: actor.Balance,
		})
	}
	
	if info.Owner != "" {
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Owner), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s owner: %s balance error: %w", info.Miner, info.Owner, err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "owner",
			Address: info.Owner,
			Balance: actor.Balance,
		})
	}
	
	if info.Worker != "" {
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Worker), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s worker: %s balance error: %w", info.Miner, info.Worker, err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "worker",
			Address: info.Worker,
			Balance: actor.Balance,
		})
	}
	
	if info.Beneficiary != "" {
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Beneficiary), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s beneficiary: %s balance error: %w", info.Miner, info.Beneficiary, err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "beneficiary",
			Address: chain.SmartAddress(info.Beneficiary).Address(),
			Balance: actor.Balance,
		})
	}
	
	if len(info.Controllers) > 0 {
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Controllers[0]), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s c0: %s balance error: %w", info.Miner, info.Controllers[0], err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "c0",
			Address: chain.SmartAddress(info.Controllers[0]).Address(),
			Balance: actor.Balance,
		})
	}
	
	if len(info.Controllers) > 1 {
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Controllers[1]), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s c1: %s balance error: %w", info.Miner, info.Controllers[1], err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "c1",
			Address: chain.SmartAddress(info.Controllers[1]).Address(),
			Balance: actor.Balance,
		})
	}
	
	if len(info.Controllers) > 2 {
		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(info.Controllers[2]), &epoch)
		if err != nil {
			err = fmt.Errorf("prepare miner: %s c2: %s balance error: %w", info.Miner, info.Controllers[2], err)
			return
		}
		items = append(items, &propo.MinerBalance{
			Epoch:   epoch.Int64(),
			Type:    "c2",
			Address: chain.SmartAddress(info.Controllers[2]).Address(),
			Balance: actor.Balance,
		})
	}
	
	return
}
