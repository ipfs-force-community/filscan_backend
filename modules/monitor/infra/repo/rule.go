package mrepo

import (
	"context"

	mpo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/po"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
)

type RuleRepo interface {
	CreateUserRule(ctx context.Context, userRules []*mpo.Rule) (err error)
	DeleteUUIDRule(ctx context.Context, userID int64, uuid string) (rowsAffected int64, err error)
	DeleteGroupIDRule(ctx context.Context, userID int64, groupID int64) (rowsAffected int64, err error)
	DeleteMinerIDRule(ctx context.Context, userID int64, minerID string) (rowsAffected int64, err error)
	SelectRulesByUserIDAndType(ctx context.Context, userID int64, typeName string) (userRules []*mpo.Rule, err error)
	UpdateActiveState(ctx context.Context, userID int64, uuid string) (rowsAffected int64, err error)
	UpdateUserMinerGroup(ctx context.Context, userID int64, minerID string, groupID int64) (rowsAffected int64, err error)
	SelectRulesByUUID(ctx context.Context, userID int64, uuid string) (userRules []*mpo.Rule, err error)
	SelectMinersByUserID(ctx context.Context, userID int64) (userMiners []*propo.UserMiner, err error)
	SelectRuleUUID(ctx context.Context, userID int64, groupID int64, miner string, mType string) (userRules *mpo.Rule, err error)
}

// 用于监控时候获取所有规则并且监控，不公开
type RuleWatcherRepo interface {
	GetAllRule(ctx context.Context) (userRules []*mpo.Rule, err error)
	SelectRulesByUserID(ctx context.Context, userID int64) (userRules []*mpo.Rule, err error)
	UpdateUserRuleVIPExpire(ctx context.Context, userID int64, isVip bool) (rowsAffected int64, err error)
}
