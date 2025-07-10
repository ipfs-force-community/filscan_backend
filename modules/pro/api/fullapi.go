package pro

import mapi "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/api"

type FullAPI interface {
	AuthAPI
	MinerAPI
	GroupAPI
	MemberShipAPI
	mapi.RuleManager
}
