package convertor

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type OwnerRewardConvertor struct {
}

func (OwnerRewardConvertor) ToOwnerRewardPo(source *owner.Reward) (target *po.OwnerRewardPo, err error) {
	
	target = &po.OwnerRewardPo{
		Epoch:         source.Epoch.Int64(),
		Owner:         source.Owner.Address(),
		Reward:        source.Reward.Decimal(),
		BlockCount:    source.BlockCount,
		AccReward:     source.AccReward.Decimal(),
		AccBlockCount: source.AccBlockCount,
		SyncMinerRef:  source.SyncMinerRef.Int64(),
		PrevEpochRef:  source.PrevEpochRef.Int64(),
		Miners:        nil,
	}
	
	for _, v := range source.Miners {
		target.Miners = append(target.Miners, v.Address())
	}
	
	return
}

func (OwnerRewardConvertor) ToOwnerRewardEntity(source *po.OwnerRewardPo) (target *owner.Reward, err error) {
	
	target = &owner.Reward{
		Epoch:         chain.Epoch(source.Epoch),
		Owner:         chain.SmartAddress(source.Owner),
		Reward:        chain.AttoFil(source.Reward),
		BlockCount:    source.BlockCount,
		PrevEpochRef:  chain.Epoch(source.PrevEpochRef),
		AccReward:     chain.AttoFil(source.AccReward),
		AccBlockCount: source.AccBlockCount,
		SyncMinerRef:  chain.Epoch(source.SyncMinerRef),
		Miners:        nil,
	}
	for _, v := range source.Miners {
		target.Miners = append(target.Miners, chain.SmartAddress(v))
	}
	
	return
}
