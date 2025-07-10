package injector

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	price_syncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/price-syncer"
)

var NewFilpriceSet = wire.NewSet(NewFilpriceTask)

func NewFilpriceTask(db *gorm.DB) *price_syncer.FilpriceTask {
	return price_syncer.NewFilpriceTask(dal.NewFilPriceDal(db))
}
