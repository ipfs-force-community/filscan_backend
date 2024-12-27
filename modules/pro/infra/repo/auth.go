package prorepo

import (
	"context"
	"time"

	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
)

type AuthRepo interface {
	SaveUser(ctx context.Context, user *propo.User) (err error)
	GetUserByMailOrNil(ctx context.Context, mail string) (user *propo.User, err error)
	GetUserById(ctx context.Context, id int64) (user *propo.User, err error)
	GetActivityStateAndSetTrue(ctx context.Context, id int64) (isActivity bool, err error)
	UpdateUserPassword(ctx context.Context, id int64, password string) (err error)
	UpdateUserName(ctx context.Context, id int64, name string) (err error)
	UpdateUserLoginTime(ctx context.Context, id int64, loginAt, lastLoginAt time.Time) (err error)
}
