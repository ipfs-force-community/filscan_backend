package prorepo

import (
	"context"

	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type GroupRepo interface {
	SelectGroupByID(ctx context.Context, ID int64) (group *propo.Group, err error)
	SelectGroupsByUserID(ctx context.Context, userID int64) (group []*propo.Group, err error)
	SelectActiveGroupsByUserID(ctx context.Context, userID int64) (group []*propo.Group, err error)
	SelectGroupMinersByUserID(ctx context.Context, userID int64) (groups []*probo.GroupMiners, err error)
	SelectGroupMinersByGroupID(ctx context.Context, groupID int64) (groups []*probo.GroupMiners, err error)
	GetUserGroupByName(ctx context.Context, userId int64, groupName string) (group *propo.Group, err error)
	CreateGroup(ctx context.Context, group *propo.Group) (err error)
	UpdateGroup(ctx context.Context, group *propo.Group) (result int64, err error)
	DeleteGroup(ctx context.Context, id int64) (err error)
	GetUserGroupNodes(ctx context.Context, userId, groupId int64) (nodes []*propo.UserMiner, err error)
}

type UserMinerRepo interface {
	SelectGroupMinersByUserID(ctx context.Context, userID int64, limit int64) (userMiner []*probo.UserMiner, err error)
	SelectGroupMinersByGroupID(ctx context.Context, userId, groupID int64) (userMiner []*probo.UserMiner, err error)
	SelectMinersByUserID(ctx context.Context, userID int64) (userMiner []*propo.UserMiner, err error)
	SelectMinersByMiners(ctx context.Context, userID int64, miners []chain.SmartAddress) (userMiner []*propo.UserMiner, err error)
	CreateUserMiner(ctx context.Context, UserMiner []*propo.UserMiner) (result []*pro.MinerInfo, err error)
	DeleteUserMinerList(ctx context.Context, groupID int64, minerIDList []chain.SmartAddress) (result []chain.SmartAddress, err error)
	DeleteUserMinerByGroupID(ctx context.Context, userId int64, groupID int64) (err error)
	DeleteUserMinerByMinerID(ctx context.Context, userID int64, minerID string) (err error)
	CountUserMiners(ctx context.Context, userId int64) (count int64, err error)
	QueryExistsMiners(ctx context.Context, userId int64, groupId int64, miners []string) (result map[string]struct{}, err error)
}
