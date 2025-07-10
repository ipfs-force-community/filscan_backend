package assembler

import (
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BlockRewardTrendAssembler struct {
}

func (BlockRewardTrendAssembler) ToBlockRewardTrendResponse(epoch chain.Epoch, items []*bo.SumMinerReward) (target *filscan.BlockRewardTrendResponse, err error) {

	target = &filscan.BlockRewardTrendResponse{
		Epoch:     epoch.Int64(),
		BlockTime: epoch.Time().String(),
		Items:     nil,
	}

	for _, v := range items {
		abr := decimal.NewFromInt(1100000000).Mul(decimal.NewFromFloat(1e18)).Sub(v.Balance)
		target.Items = append(target.Items, &filscan.BlockRewardTrend{
			BlockTime:         chain.Epoch(v.Epoch).Time().Unix(),
			AccBlockRewards:   abr,
			BlockRewardPerTib: v.AccRewardPerT,
		})
	}

	return

}
