package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
)

type OwnerRankAssembler struct {
}

func (OwnerRankAssembler) ToOwnerRankResponse(total int64, page, limit int, owners []*bo.OwnerRank) (resp *filscan.OwnerRankResponse, err error) {

	resp = &filscan.OwnerRankResponse{
		Total: total,
		Items: nil,
	}

	for index, v := range owners {
		resp.Items = append(resp.Items, &filscan.OwnerRankResponseItem{
			Rank:            limit*(page) + index + 1,
			OwnerID:         v.Owner,
			QualityAdjPower: v.QualityAdjPower,
			RewardsRatio24h: v.RewardPowerRatio,
			PowerChange24h:  v.QualityAdjPowerChange,
			BlockCount:      v.AccBlockCount,
		})
	}

	return
}
