package probiz

import (
	"github.com/gozelle/mail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	mbiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/service"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz/auth"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

func NewFullBiz(conf *config.Pro, db *gorm.DB, adapter londobell.Adapter, agg londobell.Agg, minerAgg londobell.MinerAgg, m *mail.Client, r *redis.Redis) pro.FullAPI {
	return &FullBiz{
		AuthBiz:            auth.NewAuth(conf, db, m, r),
		GroupBiz:           NewGroup(db, adapter, r),
		MinerBiz:           NewMiner(db, adapter, agg, minerAgg),
		RuleBiz:            mbiz.NewRuleBiz(db, adapter),
		MemberShipBiz:      NewMemberShipBiz(db, r, conf.VipSecret),
		CapitalAnalysisBiz: NewCapitalAnalysisBiz(db, agg, adapter, r),
	}
}

var _ pro.FullAPI = (*FullBiz)(nil)

type FullBiz struct {
	*auth.AuthBiz
	*GroupBiz
	*MinerBiz
	*MemberShipBiz
	*mbiz.RuleBiz
	*CapitalAnalysisBiz
}
