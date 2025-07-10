package injector

import (
	"github.com/google/wire"
	"github.com/gozelle/mail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	probiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

var ProSet = wire.NewSet(NewPro)

func NewPro(conf *config.Config, db *gorm.DB, adapter londobell.Adapter, agg londobell.Agg, minerAgg londobell.MinerAgg, m *mail.Client, r *redis.Redis) pro.FullAPI {
	return probiz.NewFullBiz(conf.Pro, db, adapter, agg, minerAgg, m, r)
}
