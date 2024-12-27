package mergerimpl

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

func NewMergerImpl(db *gorm.DB, adapter londobell.Adapter, agg londobell.Agg, minerAgg londobell.MinerAgg) *MergerImpl {
	hg := &heightGetter{agg: agg, adapter: adapter}
	return &MergerImpl{
		heightGetter:       hg,
		minerInfos:         minerInfos{syncer: prodal.NewSyncerDal(db), repo: prodal.NewMinerInfoDal(db), agg: agg, minerAgg: minerAgg, adapter: adapter},
		minersLuckStats:    minersLuckStats{epochGetter: dal.NewSyncEpochGetterDal(db), repo: prodal.NewLuckDal(db)},
		minersRewardStats:  minersRewardStats{repo: prodal.NewRewardDal(db), syncerGetter: dal.NewSyncerDal(db)},
		minersBalanceStats: minersBalanceStats{adapter: adapter, heightGetter: hg},
		minersFundStats:    minersFundStats{repo: prodal.NewMinerInfoDal(db)},
		minersSectorStats:  minersSectorStats{repo: prodal.NewMinerInfoDal(db), db: db},
		minersPowerStats:   minersPowerStats{repo: prodal.NewMinerInfoDal(db)},
	}
}

var _ merger.Merger = (*MergerImpl)(nil)

type MergerImpl struct {
	*heightGetter
	minerInfos
	minersPowerStats
	minersBalanceStats
	minersFundStats
	minersLuckStats
	minersSectorStats
	minersRewardStats
}
