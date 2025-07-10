package proutils

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strconv"
	"time"

	mdal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/dal"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/vip"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

const (
	year  Type = "h"
	day   Type = "d"
	month Type = "m"
)
const VipUserInfo = "VipUserInfo"

type Type string

func InternalRechargeMembership(ctx context.Context, db *gorm.DB, r *redis.Redis, userID int64, mType pro.MemberShipType, extendTime string) (expiredTime time.Time, err error) {
	err = ResolveMemberShipType(mType)
	if err != nil {
		return
	}
	//调用函数请保障事务性
	if v := ctx.Value(_dal.CONTEXT_DB_KEY); v == nil {
		tx := db.Begin()
		defer func() {
			e := recover()
			if e != nil {
				log.Println("something panic, internal recharge membership ", e)
				debug.PrintStack()
			}
			if err != nil {
				tx.Rollback()
			} else {
				tx.Commit()
			}
		}()
		ctx = _dal.ContextWithDB(ctx, tx)
	}
	msRepo := prodal.NewMemberShipDal(db)
	ruleWatcherRepo := mdal.NewRuleWatcherDal(db)
	gap, t, err := ResolveTimeType(extendTime)
	if err != nil {
		return
	}
	membership, err := msRepo.GetMemberShipByUserID(ctx, userID) //可能之前没有，这次属于第一次充值
	if err != nil {
		return
	}
	expiredTime = membership.ExpiredTime
	// 有可能充值后过期了，expire time在很早之前。不做该处理会造成过期时间错误
	if time.Now().After(expiredTime) {
		expiredTime = time.Now()
	}
	// 充值时候会多送一点点时间，让用户正好到第二天的零点。之后充值就相当于多送一天
	expiredTime = time.Date(expiredTime.Year(), expiredTime.Month(), expiredTime.Day(), 0, 0, 0, 0, expiredTime.Location()).Add(24 * time.Hour)
	switch t {
	case year:
		expiredTime = expiredTime.AddDate(gap, 0, 0)
	case month:
		expiredTime = expiredTime.AddDate(0, gap, 0)
	case day:
		expiredTime = expiredTime.AddDate(0, 0, gap)
	}
	membership.ExpiredTime = expiredTime
	membership.MemType = string(mType)
	membership.UpdatedAt = time.Now()
	err = msRepo.CreateUserMemberShip(ctx, membership)
	if err != nil {
		return
	}
	record := &propo.RechargeRecord{
		UserId:     userID,
		MemType:    string(mType),
		ExtendTime: extendTime,
		CreatedAt:  time.Now(),
	}
	err = msRepo.CreateRechargeRecord(ctx, record)
	if err != nil {
		return
	}
	_, err = ruleWatcherRepo.UpdateUserRuleVIPExpire(ctx, userID, true)
	if err != nil {
		return
	}
	rv := vip.VIP{
		UserID:      userID,
		MType:       mType,
		ExpiredTime: expiredTime,
	}
	minDuration := membership.ExpiredTime.Sub(time.Now())
	if minDuration > 7*24*time.Hour {
		minDuration = 7 * 24 * time.Hour
	}
	err = r.Set(fmt.Sprintf("%s-%d", VipUserInfo, userID), rv, minDuration)
	if err != nil {
		return
	}
	return expiredTime, nil
}

// 用于做group切片的差集
func SliceDifferenceAMinusB(a, b []int64) []int64 {
	var diff []int64
	// 创建一个映射用于存储切片 b 中的元素
	bMap := make(map[int64]struct{})
	for _, num := range b {
		bMap[num] = struct{}{}
	}
	// 遍历切片 a，检查每个元素是否存在于切片 b 中
	for _, num := range a {
		if _, ok := bMap[num]; !ok {
			diff = append(diff, num)
		}
	}
	return diff
}

// ResolveTimeType 解析时间间隔字符串
// 1y => 1, y
// 7d  => 7, d
// 12m  => 12, m
func ResolveTimeType(s string) (gap int, t Type, err error) {

	l := len(s)

	if l < 2 {
		err = fmt.Errorf("invaild interval string: %s", s)
		return
	}

	g, err := strconv.ParseInt(s[0:l-1], 10, 64)
	if err != nil {
		return
	}
	gap = int(g)
	v := Type(s[l-1:])
	switch v {
	case year, day, month:
		t = v
	default:
		err = fmt.Errorf("invaild interval type: %s", s)
		return
	}
	return
}

func ResolveMemberShipType(s pro.MemberShipType) (err error) {
	v := s
	switch v {
	case pro.EnterpriseProVIP, pro.EnterpriseVIP:

	default:
		err = fmt.Errorf("invaild membership type: %s", s)
		return
	}
	return
}
