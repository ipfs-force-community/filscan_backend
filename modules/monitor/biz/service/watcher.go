package mbiz

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	logging "github.com/gozelle/logger"
	"github.com/robfig/cron"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/notify"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/rule"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/convertor"
	mdal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/dal"
	mpo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/po"
	mrepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/repo"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

const BaseTickInterval = 30
const VipUserInfo = "VipUserInfo"

func NewWatcherBiz(db *gorm.DB, adapter londobell.Adapter, redis *redis.Redis) *WatcherBiz {
	return &WatcherBiz{
		ruleWatcherRepo: mdal.NewRuleWatcherDal(db),
		memberShipRepo:  prodal.NewMemberShipDal(db),
		authRepo:        prodal.NewAuthDal(db),
		db:              db,
		adapter:         adapter,
		redis:           redis,
	}
}

var logger = logging.NewLogger("watcher")
var existUUIDMap map[string]int

type WatcherBiz struct {
	ruleWatcherRepo mrepo.RuleWatcherRepo
	memberShipRepo  prorepo.MemberShipRepo
	authRepo        prorepo.AuthRepo
	db              *gorm.DB
	redis           *redis.Redis
	adapter         londobell.Adapter
	live            uint64
	clock           *time.Ticker
}

func (rw *WatcherBiz) RuleWatch(ctx context.Context) {
	rw.clock = time.NewTicker(BaseTickInterval * time.Second)
	// 建立一个全局的map来保存所有规则的时间相关的
	go func() {
		for {
			select {
			case <-rw.clock.C:
				now := time.Now()
				rw.live = rw.live + BaseTickInterval
				rules, err := rw.ruleWatcherRepo.GetAllRule(ctx)
				if err != nil {
					return
				}
				// 将所有的数据库po转换成内存中要使用的部分
				rc := convertor.RuleConvertor{}
				ruleList := rc.ToRuleList(rules)
				existUUIDMap = make(map[string]int)
				// 每个循环给个管道用于收集当下的状态
				var ch = make(chan *rule.UuidExeRes, len(ruleList))
				var wg sync.WaitGroup
				for _, r := range ruleList {
					existUUIDMap[r.GetUUID()] = 0
				}
				cnt := 0
				for _, r := range ruleList {
					existUUIDMap[r.GetUUID()]++
					if existUUIDMap[r.GetUUID()] == 1 {
						checkOrNewNotifyInfo(ctx, r)
					}
					if r.GetInterval() > 0 && rw.live%(uint64)(r.GetInterval()) == 0 {
						// 判断是否是之前有报警过，过滤掉暂时不报警的
						info := global.GetGlobalNotifyInfoMap(ctx, r.GetUUID())
						if r.GetIsActive() && time.Now().After(info.AllowDitheringTime) {
							wg.Add(1)
							cnt++
							go func(ctx context.Context, r rule.ActionOfRule, ch chan<- *rule.UuidExeRes) {
								defer wg.Done()
								r.Evaluate(ctx, ch)
							}(ctx, r, ch)
						}
					}
				} //在这里结束后统一收集所有uuid执行的判断，判断是否要重置
				go func() {
					for res := range ch {
						if !res.IsAbnormal {
							existUUIDMap[res.Uuid]--
						}
					}
					checkAlarmReset(ctx, existUUIDMap)
				}()
				wg.Wait()
				close(ch)
				logger.Infof("evaluate %d rules, once monitor execution total time: %s\n", cnt, time.Now().Sub(now))
			}
		}
	}()
}

func (rw *WatcherBiz) VipWatch(ctx context.Context) {
	// 创建定时任务对象
	c := cron.New()
	// 添加定时任务
	fmt.Println(time.Now().String())
	err := c.AddFunc("0 1 0 * * *", func() {
		rw.CheckVipExpiration(ctx)
		rw.RemoveOutdatedRule(ctx) //定期清理过期的规则

	}) // 每天零点零一分执行任务
	if err != nil {
		return
	}

	err = c.AddFunc("0 30 10 * * *", func() {
		rw.SendVipExpirationNotification(ctx)
	}) // 十点半检查会员相关信息，然后发送通知信息
	if err != nil {
		return
	}
	// 启动定时任务
	c.Start()
	// 阻塞主线程，保持定时任务运行
	select {}
}

func (rw *WatcherBiz) permissionRevocation(ctx context.Context, userID int64) (err error) {
	tx := rw.db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	ctx = _dal.ContextWithDB(ctx, tx)
	// 将会员信息表中的会员类型改回NormalVIP
	rowsAffected, err := rw.memberShipRepo.UpdateUserVIPExpire(ctx, userID)
	if err != nil {
		return
	}
	if rowsAffected == 0 { //保证一定会成功
		return fmt.Errorf("userID %d memberShipRepo.UpdateUserVIPExpire error", userID)
	}
	// 将该用户规则中的vip置为false
	_, err = rw.ruleWatcherRepo.UpdateUserRuleVIPExpire(ctx, userID, false) //可能不存在规则
	if err != nil {
		return
	}
	_, err = rw.redis.Delete(fmt.Sprintf("%s-%d", VipUserInfo, userID))
	if err != nil {
		return
	}
	return nil
}

func (rw *WatcherBiz) CheckVipExpiration(ctx context.Context) {
	memberships, err := rw.memberShipRepo.GetAllUserMemberShip(ctx)
	if err != nil {
		logger.Errorln("memberShipRepo.GetAllUserMemberShip error", err)
	}
	for _, membership := range memberships {
		if pro.MemberShipType(membership.MemType) != pro.NormalVIP {
			//判断会员有没有过期，过期后取消所有权限
			if time.Now().After(membership.ExpiredTime) {
				err = rw.permissionRevocation(ctx, membership.UserId)
				if err != nil {
					logger.Errorln(membership.UserId, "permissionRevocation error", err) //往上抛出没意义，也不能终止程序。那就放日志里看看吧
				}
				logger.Infoln(membership.UserId, "vip expiration, permission revocation success")
			}
		}
	}
}

const mailTemplateVip = `您的会员还有${day}天到期，请及时续费，以免影响业务正常使用。`
const msgTemplateVip = `您的会员还有${day}天到期，请及时续费，以免影响业务正常使用。`
const msgTemplateCodeVip = "SMS_463915671"

var NotityDay = []int64{1, 7, 15}

func (rw *WatcherBiz) SendVipExpirationNotification(ctx context.Context) {
	memberships, err := rw.memberShipRepo.GetAllUserMemberShip(ctx)
	if err != nil {
		logger.Errorln("memberShipRepo.GetAllUserMemberShip error", err)
	}
	for _, membership := range memberships {
		if pro.MemberShipType(membership.MemType) != pro.NormalVIP {
			//判断会员是否1天、7天、15天到期，是则发消息
			for _, day := range NotityDay {
				now := time.Now()
				dday := time.Duration(day + 1) //提醒时间是比day天早一点发送
				if time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(dday * 24 * time.Hour).Equal(membership.ExpiredTime) {
					if time.Now().Sub(membership.CreatedAt) < 24*time.Hour {
						continue //避免刚注册就发送到期提醒
					}
					var numbers []string
					//获取该用户所有的规则
					userRules, err := rw.ruleWatcherRepo.SelectRulesByUserID(ctx, membership.UserId)
					if err != nil {
						logger.Errorln("会员到期提醒：用户ID-", membership.UserId, " 获取用户规则失败！", err)
					}
					user, err := rw.authRepo.GetUserById(ctx, membership.UserId)
					if err != nil {
						logger.Errorln("会员到期提醒：用户ID-", membership.UserId, " 获取用户相关信息失败！", err)
					}
					formatInt := strconv.FormatInt(day, 10)
					replaceMailMsg := strings.ReplaceAll(mailTemplateVip, "${day}", formatInt)
					err = notify.GlobalNotify.AlertMail.Send([]string{user.Mail}, replaceMailMsg, []string{"会员到期提醒"}) //发送邮件
					if err != nil {
						logger.Errorln("会员到期提醒：用户ID-", membership.UserId, " 发送提醒邮件失败！", err)
					}
					if len(userRules) != 0 {
						//获取规则里的电话号
						numbers = getUserPhoneNumbers(userRules)
						if len(numbers) != 0 {
							str := fmt.Sprintf("{\"day\":\"%s\"}", formatInt)
							err = notify.GlobalNotify.AlertALiMsg.Send(numbers, msgTemplateCodeVip, []string{str})
							if err != nil {
								logger.Errorln("会员到期提醒：用户ID-", membership.UserId, " 发送短信提醒失败！", err)
							}
						}
					}
				}
			}
		}
	}
}

func getUserPhoneNumbers(userRules []*mpo.Rule) (phoneNumbers []string) {
	var buffer bytes.Buffer
	for _, userRule := range userRules {
		if userRule.MsgAlert != nil {
			if buffer.Len() == 0 {
				buffer.WriteString(*userRule.MsgAlert)
			} else {
				buffer.WriteString("," + *userRule.MsgAlert)
			}
		}
	}
	return combinePhoneNumber(buffer.String())
}

func combinePhoneNumber(receiversStr string) (phoneNumbers []string) {
	receivers, err := rc.SplitReceiver(receiversStr)
	if err != nil {
		return
	}
	m := make(map[string]struct{})
	for _, receiver := range receivers {
		if _, ok := m[receiver]; !ok {
			m[receiver] = struct{}{}
			phoneNumbers = append(phoneNumbers, receiver)
		}
	}
	return phoneNumbers
}

func (rw *WatcherBiz) RemoveOutdatedRule(ctx context.Context) {
	rules, err := rw.ruleWatcherRepo.GetAllRule(ctx)
	if err != nil {
		return
	}
	removeUUIDMap := make(map[string]int)
	for _, r := range rules {
		removeUUIDMap[r.Uuid] = 0
	}
	deleteDuplicateRule(ctx, removeUUIDMap)
}

func checkAlarmReset(ctx context.Context, existUUIDMap map[string]int) {
	now := time.Now()
	err := global.Lock.Lock()
	if err != nil {
		logger.Errorf("checkAlarmReset redis set lock error: %v", err)
		return
	}
	defer func() {
		if ok, err := global.Lock.Unlock(); !ok || err != nil {
			logger.Errorf("checkAlarmReset redis release lock error: %v", err)
			logger.Debugln("============================== checkAlarmReset lock time:", time.Now().Sub(now))
			return
		}
		logger.Debugln("============================== checkAlarmReset lock time:", time.Now().Sub(now))
	}()
	for uuid, i := range existUUIDMap {
		info := global.GetGlobalNotifyInfoMapWithoutLock(ctx, uuid)
		if 0 == i && info.IsPreAbnormal { //正常时候计数为0
			info.IsPreAbnormal = false
			info.NotifyCount = 0
			info.NotifyTime = time.Now()
			info.AllowDitheringTime = time.Now()
			global.SetGlobalNotifyInfoMapWithoutLock(ctx, uuid, info)
			logger.Infoln("UUID-", uuid+"【状态恢复，解除告警】")
		}
	}
}

func deleteDuplicateRule(ctx context.Context, existUUIDMap map[string]int) {
	now := time.Now()
	err := global.Lock.Lock()
	if err != nil {
		logger.Errorf("deleteDuplicateRule redis set lock error: %v", err)
		return
	}
	defer func() {
		if ok, err := global.Lock.Unlock(); !ok || err != nil {
			logger.Errorf("deleteDuplicateRule redis release lock error: %v", err)
			logger.Debugln("============================== deleteDuplicateRule lock time:", time.Now().Sub(now))
			return
		}
		logger.Debugln("============================== deleteDuplicateRule lock time:", time.Now().Sub(now))

	}()
	keys, err := global.Global.Redis.Keys("UUID*")
	if err != nil {
		logger.Errorf("get keys from redis error: %v", err)
		return
	}
	for _, key := range keys {
		//去掉前面用于获取pattern的前缀
		uuid := strings.TrimPrefix(key, "UUID-")
		if _, ok := existUUIDMap[uuid]; !ok {
			isDelete, err := global.Global.Redis.Delete(key)
			if err != nil {
				logger.Errorf("delete uuid %s from redis error: %v", uuid, err)
				return
			}
			if isDelete {
				logger.Infoln("uuid-", uuid, "【规则从全局map中删除】")
			}
		}
	}
}

func checkOrNewNotifyInfo(ctx context.Context, r rule.ActionOfRule) {
	now := time.Now()
	err := global.Lock.Lock()
	if err != nil {
		logger.Debugln("checkOrNewNotifyInfo redis set lock error: %v", err)
		return
	}
	defer func() {
		if ok, err := global.Lock.Unlock(); !ok || err != nil {
			logger.Errorf("checkOrNewNotifyInfo redis release lock error: %v", err)
			logger.Debugln("============================== checkOrNewNotifyInfo lock time:", time.Now().Sub(now))
			return
		}
		logger.Debugln("============================== checkOrNewNotifyInfo lock time:", time.Now().Sub(now))
	}()
	// 如果更新了规则，则重置报警状态
	info := global.GetGlobalNotifyInfoMapWithoutLock(ctx, r.GetUUID())
	if info == nil {
		info = &global.NotifyInfo{
			IsPreAbnormal:      false,
			NotifyCount:        0,
			NotifyTime:         time.Now(),
			UpdateAt:           r.GetUpdateAt(),
			AllowDitheringTime: time.Now(),
		}
		global.SetGlobalNotifyInfoMapWithoutLock(ctx, r.GetUUID(), info)
		logger.Infoln(r.GetUUID(), r.GetDescription()+"【规则初次加入全局map中】")
	} else if info.UpdateAt != r.GetUpdateAt() {
		info = &global.NotifyInfo{
			IsPreAbnormal:      false,
			NotifyCount:        0,
			NotifyTime:         time.Now(),
			UpdateAt:           r.GetUpdateAt(),
			AllowDitheringTime: time.Now(),
		}
		global.SetGlobalNotifyInfoMapWithoutLock(ctx, r.GetUUID(), info)
		logger.Infoln(r.GetUUID(), r.GetDescription()+"【发生规则更新，重置告警状态】")
	}
}
