package injector

import (
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/ipquery"
	"gorm.io/gorm"
)

var MinerLocationSet = wire.NewSet(NewMinerLocationTask)

func NewMinerLocationTask(db *gorm.DB) *ipquery.MinerLocationTask {
	return ipquery.NewMinerLocationTask(dal.NewMinerLocationDal(db))
}
