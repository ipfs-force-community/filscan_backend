package injector

import (
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
)

var RedisSet = wire.NewSet(redis.NewRedis)
