package prorepo

import (
	"context"

	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
)

type MemberShipRepo interface {
	GetMemberShipByUserID(ctx context.Context, userID int64) (membership *propo.MemberShip, err error)
	GetAllUserMemberShip(ctx context.Context) (memberships []*propo.MemberShip, err error)
	CreateUserMemberShip(ctx context.Context, membership *propo.MemberShip) (err error)
	CreateRechargeRecord(ctx context.Context, record *propo.RechargeRecord) (err error)
	UpdateUserVIPExpire(ctx context.Context, userID int64) (rowsAffected int64, err error)
}
