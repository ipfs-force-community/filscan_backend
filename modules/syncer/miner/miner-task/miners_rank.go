package miner_task

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"sort"
)

var _ sort.Interface = (*QualityAdjPowerMinersRank)(nil)

type QualityAdjPowerMinersRank []*po.MinerInfo

func (q QualityAdjPowerMinersRank) Len() int {
	return len(q)
}

func (q QualityAdjPowerMinersRank) Less(i, j int) bool {
	return q[i].QualityAdjPower.GreaterThan(q[j].QualityAdjPower)
}

func (q QualityAdjPowerMinersRank) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}
