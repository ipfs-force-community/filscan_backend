package browser

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gorm.io/gorm"
)

func NewRankBiz(db *gorm.DB, conf *config.Config) *RankBiz {
	return &RankBiz{
		OwnerRankBiz: NewOwnerRankBiz(dal.NewSyncEpochGetterDal(db), dal.NewOwnerRankBizDal(db)),
		MinerRankBiz: NewMinerRankBiz(dal.NewSyncEpochGetterDal(db), dal.NewMinerRankBizDal(db), conf),
	}
}

var _ filscan.RankAPI = (*RankBiz)(nil)

type RankBiz struct {
	*OwnerRankBiz
	*MinerRankBiz
}
