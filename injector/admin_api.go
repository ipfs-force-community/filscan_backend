package injector

import (
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	syncer_api "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/biz"
)

var AdminAPISet = wire.NewSet(NewAdminAPI)

func NewAdminAPI(m *syncer.Manager) syncer_api.AdminAPI {
	return biz.NewAdminBiz(m)
}
