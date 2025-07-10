package injector

import (
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/fullactors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

var FullActorsSet = wire.NewSet(NewFullActors)

func NewFullActors(db *gorm.DB, agg londobell.Agg) *fullactors.Syncer {
	return fullactors.NewSyncer(dal.NewActorSyncSaver(db), agg)
}
