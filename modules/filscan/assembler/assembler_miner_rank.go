package assembler

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
)

type MinerRankAssembler struct {
}

func (MinerRankAssembler) ToMinerRankResponse(total int64, page, limit int, ranks []*bo.MinerRank) (target *filscan.MinerRankResponse, err error) {

	target = &filscan.MinerRankResponse{
		Total: total,
		Items: nil,
	}

	for index, v := range ranks {
		target.Items = append(target.Items, &filscan.MinerRank{
			Rank:              limit*(page) + index + 1,
			MinerID:           v.Miner,
			Balance:           v.Balance,
			QualityAdjPower:   v.QualityAdjPower,
			QualityPowerRatio: v.QualityAdjPowerPercent,
			PowerIncrease24H:  v.QualityAdjPowerChange,
			BlockCount:        v.AccBlockCount,
			BlockRatio:        v.AccBlockCountPercent,
			Rewards:           v.AccReward,
			RewardsRatio:      v.AccRewardPercent,
		})
	}

	return
}

func (MinerRankAssembler) ToMinerPowerRankResponse(total int64, page, limit int, ranks []*bo.MinerPowerRank) (target *filscan.MinerPowerRankResponse, err error) {

	target = &filscan.MinerPowerRankResponse{
		Total: total,
		Items: nil,
	}
	for index, v := range ranks {
		if v.Epoch-v.PrevEpoch == 0 {
			err = fmt.Errorf("calc power ratio error, please check")
			return
		}
		target.Items = append(target.Items, &filscan.MinerPowerRank{
			Rank:                 limit*(page) + index + 1,
			MinerID:              v.Miner,
			PowerRatio:           v.QualityAdjPowerChange.Div(decimal.NewFromInt((v.Epoch - v.PrevEpoch) / 2880)),
			RawPower:             v.RawBytePower,
			QualityAdjPower:      v.QualityAdjPower,
			QualityPowerIncrease: v.QualityAdjPowerChange,
			SectorSize:           humanize.IBytes(uint64(v.SectorSize)),
		})
	}

	return
}

func (MinerRankAssembler) ToMinerRewardRankResponse(total int64, page, limit int, ranks []*bo.MinerRewardRank) (target *filscan.MinerRewardRankResponse, err error) {

	target = &filscan.MinerRewardRankResponse{
		Total: total,
		Items: nil,
	}

	for index, v := range ranks {
		target.Items = append(target.Items, &filscan.MinerRewardRank{
			Rank:            limit*(page) + index + 1,
			MinerID:         v.Miner,
			BlockCount:      v.AccBlockCount,
			Rewards:         v.AccReward,
			RewardsRatio:    v.AccRewardPercent,
			WinningRate:     v.WiningRate,
			QualityAdjPower: v.QualityAdjPower,
			SectorSize:      humanize.IBytes(uint64(v.SectorSize)),
		})
	}

	return
}
