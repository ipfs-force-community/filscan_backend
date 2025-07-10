package reward_task

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/debuglog"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewMinerRewardTask(repo repository.RewardTask) *MinerRewardTask {
	return &MinerRewardTask{repo: repo}
}

var _ syncer.Task = (*MinerRewardTask)(nil)

type MinerRewardTask struct {
	repo repository.RewardTask
}

func (m MinerRewardTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (m MinerRewardTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = m.repo.DeleteOwnerRewards(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteMinerRewards(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteWinCounts(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (m MinerRewardTask) save(ctx context.Context, minerRewards []*miner.Reward, ownerRewards []*owner.Reward, winCounts []*po.MinerWinCount) (err error) {

	if len(minerRewards) > 0 {
		err = m.repo.SaveMinerRewards(ctx, minerRewards)
		if err != nil {
			return
		}
	}

	if len(ownerRewards) > 0 {
		err = m.repo.SaveOwnerRewards(ctx, ownerRewards)
		if err != nil {
			return
		}
	}

	if len(winCounts) > 0 {
		err = m.repo.SaveWinCounts(ctx, winCounts)
		if err != nil {
			return
		}
	}

	return
}

func (m MinerRewardTask) Name() string {
	return "reward-task"
}

func (m MinerRewardTask) Exec(ctx *syncer.Context) (err error) {
	if ctx.Empty() {
		return
	}

	rewards, err := ctx.Agg().MinersBlockReward(ctx.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}

	winCounts, err := ctx.Agg().WinCount(ctx.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}
	debuglog.Logger.Infof("reward task, epoch: %d, miner rewards: %d, win counts: %d", ctx.Epoch(), len(rewards), len(winCounts))

	var minerRewards []*miner.Reward
	for _, v := range rewards {
		minerRewards = append(minerRewards, m.toMinerRewardEntity(v))
	}

	var ownerRewards []*owner.Reward
	ownerRewards, err = m.prepareOwnerRewards(ctx, minerRewards)
	if err != nil {
		return
	}

	var minerWinCounts []*po.MinerWinCount
	for _, v := range winCounts {
		minerWinCounts = append(minerWinCounts, &po.MinerWinCount{
			Epoch:    ctx.Epoch().Int64(),
			Miner:    chain.SmartAddress(v.Id).Address(),
			WinCount: v.TotalWinCount,
		})
	}

	err = m.save(ctx.Context(), minerRewards, ownerRewards, minerWinCounts)
	if err != nil {
		return
	}

	return
}

// 通过节点查询当前高度 miner 的 owner
func (m MinerRewardTask) getMinerOwner(ctx *syncer.Context, miner chain.SmartAddress) (owner chain.SmartAddress, err error) {

	epoch := ctx.Epoch()
	reply, err := ctx.Adapter().Miner(ctx.Context(), miner, &epoch)
	if err != nil {
		return
	}

	owner = chain.SmartAddress(reply.Owner)

	return
}

func (m MinerRewardTask) prepareOwnerRewards(ctx *syncer.Context, minerRewards []*miner.Reward) (ownersRewards []*owner.Reward, err error) {

	rewardsMap := map[string]*owner.Reward{}
	for _, v := range minerRewards {
		var ownerAddr chain.SmartAddress
		ownerAddr, err = m.getMinerOwner(ctx, v.Miner)
		if err != nil {
			return
		}
		if _, ok := rewardsMap[ownerAddr.Address()]; !ok {
			rewardsMap[ownerAddr.Address()] = &owner.Reward{
				Epoch:        ctx.Epoch(),
				Owner:        ownerAddr,
				SyncMinerRef: ctx.Epoch(),
				PrevEpochRef: ctx.Epoch(),
			}
		}

		rewardsMap[ownerAddr.Address()].Reward = chain.AttoFil(rewardsMap[ownerAddr.Address()].Reward.Decimal().Add(v.Reward.Decimal()))
		rewardsMap[ownerAddr.Address()].BlockCount = rewardsMap[ownerAddr.Address()].BlockCount + v.BlockCount
		rewardsMap[ownerAddr.Address()].AccReward = chain.AttoFil(rewardsMap[ownerAddr.Address()].AccReward.Decimal().Add(v.Reward.Decimal()))
		rewardsMap[ownerAddr.Address()].AccBlockCount = rewardsMap[ownerAddr.Address()].AccBlockCount + v.BlockCount
		rewardsMap[ownerAddr.Address()].Miners = append(rewardsMap[ownerAddr.Address()].Miners, v.Miner)

		var last *owner.Reward
		last, err = m.repo.GetLastOwnerRewardOrNil(ctx.Context(), ctx.Epoch(), ownerAddr)
		if err != nil {
			return
		}
		if last != nil {
			rewardsMap[ownerAddr.Address()].AccReward = chain.AttoFil(rewardsMap[ownerAddr.Address()].AccReward.Decimal().Add(last.AccReward.Decimal()))
			rewardsMap[ownerAddr.Address()].AccBlockCount = rewardsMap[ownerAddr.Address()].AccBlockCount + last.AccBlockCount
			rewardsMap[ownerAddr.Address()].PrevEpochRef = last.Epoch
		}
	}

	for _, v := range rewardsMap {
		ownersRewards = append(ownersRewards, v)
	}

	return
}

func (m MinerRewardTask) toMinerRewardEntity(source *londobell.MinersBlockReward) (target *miner.Reward) {
	target = &miner.Reward{
		Epoch:      chain.Epoch(source.Id.Epoch),
		Miner:      chain.SmartAddress(source.Id.Miner),
		Reward:     chain.AttoFil(source.TotalBlockReward),
		BlockCount: source.BlockCount,
	}
	return target
}
