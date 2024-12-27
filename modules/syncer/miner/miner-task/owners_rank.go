package miner_task

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"sort"
)

var _ sort.Interface = (*QualityAdjPowerOwnersRank)(nil)

type QualityAdjPowerOwnersRank []*po.OwnerInfo

func (q QualityAdjPowerOwnersRank) Len() int {
	return len(q)
}

func (q QualityAdjPowerOwnersRank) Less(i, j int) bool {
	return q[i].QualityAdjPower.GreaterThan(q[j].QualityAdjPower)
}

func (q QualityAdjPowerOwnersRank) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}
