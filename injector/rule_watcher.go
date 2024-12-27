package injector

import (
	"github.com/google/wire"
	mbiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/service"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

var WatcherSet = wire.NewSet(NewWatcherBiz)

func NewWatcherBiz(db *gorm.DB, adapter londobell.Adapter, redis *redis.Redis) *mbiz.WatcherBiz {
	return mbiz.NewWatcherBiz(db, adapter, redis)
}
