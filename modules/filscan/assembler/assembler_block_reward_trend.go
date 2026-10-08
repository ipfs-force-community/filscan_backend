package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BlockRewardTrendAssembler struct {
}

// ToBlockRewardTrendResponse 把 DAL 取回的「累计矿工实收」点组装成曲线响应。
//
// 数值直接来自 DAL（SQLBlockRewardTrend 已按 v18/v19 分支算好矿工实收），
// **不再做 `1,100,000,000 FIL − f02 账户余额`**：那条式子等于 f02 累计出账，
// NV29 后含服务流+销毁，与「累计发给矿工的奖励」口径脱钩。
func (BlockRewardTrendAssembler) ToBlockRewardTrendResponse(epoch chain.Epoch, items []*bo.SumMinerReward) (target *filscan.BlockRewardTrendResponse, err error) {

	target = &filscan.BlockRewardTrendResponse{
		Epoch:     epoch.Int64(),
		BlockTime: epoch.Time().String(),
		Items:     nil,
	}

	for _, v := range items {
		target.Items = append(target.Items, &filscan.BlockRewardTrend{
			BlockTime:         chain.Epoch(v.Epoch).Time().Unix(),
			AccBlockRewards:   v.AccBlockRewards,
			BlockRewardPerTib: v.AccRewardPerT,
		})
	}

	return

}
