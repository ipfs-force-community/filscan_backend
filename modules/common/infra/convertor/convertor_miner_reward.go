package convertor

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type MinerRewardConvertor struct {
}

func (MinerRewardConvertor) ToMinerRewardPo(source *miner.Reward) (target *po.MinerRewardPo, err error) {
	target = &po.MinerRewardPo{
		Epoch:      source.Epoch.Int64(),
		BlockTime:  source.Epoch.Time(),
		Miner:      source.Miner.Address(),
		Reward:     source.Reward.Decimal(),
		BlockCount: source.BlockCount,
		//AccReward:     source.AccReward.Decimal(),
		//AccBlockCount: source.AccBlockCount,
		//PrevRewardRef: source.PrevRewardRef.Int64(),
	}
	return
}

func (MinerRewardConvertor) ToMinerRewardEntity(source *po.MinerRewardPo) (target *miner.Reward, err error) {
	target = &miner.Reward{
		Epoch:      chain.Epoch(source.Epoch),
		Miner:      chain.SmartAddress(source.Miner),
		Reward:     chain.AttoFil(source.Reward),
		BlockCount: source.BlockCount,
		//AccReward:     chain.AttoFil(source.AccReward),
		//AccBlockCount: source.AccBlockCount,
		//PrevRewardRef: chain.Epoch(source.PrevRewardRef),
	}
	return
}
