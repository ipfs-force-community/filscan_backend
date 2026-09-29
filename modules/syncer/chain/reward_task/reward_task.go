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
		minerWinCounts = append(minerWinCounts, toMinerWinCount(ctx.Epoch(), v))
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

		r, exist := rewardsMap[ownerAddr.Address()]
		if !exist {
			r = &owner.Reward{
				Epoch:        ctx.Epoch(),
				Owner:        ownerAddr,
				SyncMinerRef: ctx.Epoch(),
				PrevEpochRef: ctx.Epoch(),
			}

			// 上一条 owner_rewards 的累计基准**每个 owner 只能取一次**：
			// 本循环是按矿工遍历的，同一 owner 在本高度可能有 N 个爆块矿工，
			// 若把「取上一条并累加 acc」写在循环体内，上一条 acc_reward /
			// acc_block_count 会被重复累加 N 次 ⇒ 每次调用把累计值放大 N 倍
			// （连续高度上呈指数级放大，且 acc_block_count 会 int64 溢出为负数）。
			var last *owner.Reward
			last, err = m.repo.GetLastOwnerRewardOrNil(ctx.Context(), ctx.Epoch(), ownerAddr)
			if err != nil {
				return
			}
			if last != nil {
				r.AccReward = last.AccReward
				r.AccBlockCount = last.AccBlockCount
				r.PrevEpochRef = last.Epoch
			}

			rewardsMap[ownerAddr.Address()] = r
		}

		r.Reward = chain.AttoFil(r.Reward.Decimal().Add(v.Reward.Decimal()))
		r.BlockCount = r.BlockCount + v.BlockCount
		r.AccReward = chain.AttoFil(r.AccReward.Decimal().Add(v.Reward.Decimal()))
		r.AccBlockCount = r.AccBlockCount + v.BlockCount
		r.Miners = append(r.Miners, v.Miner)
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

// toMinerWinCount 把聚合器 wincount 的一行落成 chain.miner_win_counts 的 PO。
//
// TotalWinCount 与 TotalGasReward 来自**同一次** ctx.Agg().WinCount(epoch, epoch+1) 调用
// （同一个 /aggregators/wincount 响应，管线 londobell-aggregators/pool-monitor/wincount_zl.js）：
// 补 gas_reward 这一列不需要新增数据源、不需要多发一次聚合器请求，纯粹是把已经在手里的字段落盘。
// 在此之前该字段被丢弃，导致 PG 侧恒为 0（见 agg_pg_reward.go 的历史注释）。
//
// 地址形态：聚合器 _id 是 Message.Detail.Params.Miner（库内为不带前缀的 0… 形态），
// 落库统一成带前缀的 f0…（与 miner_win_counts 既有行、以及 miner_rewards 的口径一致）。
// 金额：TotalGasReward 是 decimal128 的 attoFIL，原样落 numeric，不做单位换算。
func toMinerWinCount(epoch chain.Epoch, source *londobell.MinerWinCount) *po.MinerWinCount {
	return &po.MinerWinCount{
		Epoch:     epoch.Int64(),
		Miner:     chain.SmartAddress(source.Id).Address(),
		WinCount:  source.TotalWinCount,
		GasReward: source.TotalGasReward,
	}
}
