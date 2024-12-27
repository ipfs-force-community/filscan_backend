package probiz

import (
	"context"
	"fmt"

	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz/proutils"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

const (
	VipSecretStr = "filscan VIP的充值key，注意保密"
	VipSecret    = "4bf4ac3c9a61b3174d63b33849dbb0e336c8e08a42e889bc8b9cfd4293512663"
)

type MemberShipBiz struct {
	db     *gorm.DB
	redis  *redis.Redis
	msRepo prorepo.MemberShipRepo
}

func NewMemberShipBiz(db *gorm.DB, redis *redis.Redis) *MemberShipBiz {
	return &MemberShipBiz{db: db, msRepo: prodal.NewMemberShipDal(db), redis: redis}
}

func (m *MemberShipBiz) RechargeMembership(ctx context.Context, req pro.RechargeMembershipRequest) (resp *pro.RechargeMembershipResponse, err error) {
	b := bearer.UseBearer(ctx)
	// 要求只有指定的邮箱和秘钥才能进行充值
	if b.Mail != "meiwu@kunyaokeji.com" && b.Mail != "yujinshicn@163.com" {
		return nil, fmt.Errorf("user permission error")
	}
	if req.HashKey != VipSecret {
		return nil, fmt.Errorf("key permission error")
	}
	expiredTime, err := proutils.InternalRechargeMembership(ctx, m.db, m.redis, req.UserID, req.MType, req.ExtendTime)
	if err != nil {
		return nil, err
	}
	resp = &pro.RechargeMembershipResponse{
		MType:       req.MType,
		ExpiredTime: expiredTime,
	}
	return resp, nil
}
