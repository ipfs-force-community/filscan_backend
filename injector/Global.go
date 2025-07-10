package injector

import (
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

var GlobalSet = wire.NewSet(NewGlobalBiz)

func NewGlobalBiz(conf *config.Config, db *gorm.DB, adapter londobell.Adapter, redis *redis.Redis) *global.GlobalBiz {
	return global.NewGlobalBiz(conf, db, adapter, redis)
}
