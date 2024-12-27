package vip

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gozelle/gin"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

// MemberShipInfo 以下信息会随着ctx进行传递，用于判断会员相关的状态和数据，该数据不来自于token
type VIP struct {
	UserID      int64
	Mail        string
	MType       pro.MemberShipType
	ExpiredTime time.Time
}
type ck string

const vipKey ck = "vipInfo"
const VipUserInfo = "VipUserInfo"

var whiteList = map[string]struct{}{
	"/pro/v1/Login":                  {},
	"/pro/v1/SendVerificationCode":   {},
	"/pro/v1/MailExists":             {},
	"/pro/v1/ResetPasswordByCode":    {},
	"/pro/v1/ValidInvite":            {},
	"/pro/v1/CapitalAddrTransaction": {},
	"/pro/v1/CapitalAddrInfo":        {},
	"/pro/v1/EvaluateAddr":           {},
}

var blackList = map[string]struct{}{ //哪些地址需要验证vip身份，其他的只需要将会员相关信息给放进去就好。为了高效，一般从缓存中拿，没有的话从数据库中拿再放到缓存中。
	"/pro/v1/SaveUserRules":         {},
	"/pro/v1/GetUserRules":          {},
	"/pro/v1/GetRuleMinerInfo":      {},
	"/pro/v1/DeleteUserRule":        {},
	"/pro/v1/UpdateRuleActiveState": {},
}

func UseVIP(ctx context.Context) *VIP {
	b := ctx.Value(vipKey)
	if b == nil {
		panic("cant't fetch vip by context key")
	}
	v, ok := b.(*VIP)
	if !ok {
		panic("assert *VIP failed")
	}
	return v
}

func AuthenticationWithVIP(db *gorm.DB, r *redis.Redis) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := whiteList[c.Request.URL.Path]; ok {
			return
		}
		b := bearer.UseBearer(c.Request.Context())
		v := new(VIP)
		val, err := r.GetCacheResult(fmt.Sprintf("%s-%d", VipUserInfo, b.Id))
		if err != nil {
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		// 缓存策略：在充值的界面给注册会员相关缓存，所有人（包括不是VIP的用户）都缓存，区别VIP缓存时间为有效期吧（或者24h），普通用户1h
		if val != nil {
			err = json.Unmarshal(val, v)
			if err != nil {
				_ = c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
		} else {
			msRepo := prodal.NewMemberShipDal(db)
			membership, err := msRepo.GetMemberShipByUserID(c.Request.Context(), b.Id)
			if err != nil {
				_ = c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
			v = &VIP{
				UserID:      b.Id,
				Mail:        b.Mail,
				MType:       pro.MemberShipType(membership.MemType),
				ExpiredTime: membership.ExpiredTime,
			}
			var minDuration time.Duration
			if v.MType == pro.NormalVIP {
				minDuration = time.Hour
			} else {
				minDuration = v.ExpiredTime.Sub(time.Now())
				if minDuration > 7*24*time.Hour {
					minDuration = 7 * 24 * time.Hour // 超过七天的统一缓存七天
				}
			}
			err = r.Set(fmt.Sprintf("%s-%d", VipUserInfo, b.Id), v, minDuration)
			if err != nil {
				_ = c.AbortWithError(http.StatusInternalServerError, err)
				return
			}

		}
		v.Mail = b.Mail //在充值时候，mail不一定传递到redis里了
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), vipKey, v))
		if _, ok := blackList[c.Request.URL.Path]; !ok {
			return //不在里面的直接放过，在下面的来鉴权
		}
		if v.MType == pro.NormalVIP { //必须要VIP才能访问
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
}
