package rule

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BalanceRule struct {
	BaseRule
	AccountType *string //在余额部分用于区分是owner、worker还是什么东西
	AccountAddr *string
}

var balanceSingleFlight = new(singleflight.Group)

const mailTemplateBalance = `您好，您的节点$group_name $miner_id$miner_tag $description，请注意余额变化，以免影响业务运作。`
const msgTemplateBalance = `您的节点${minerid}的${item}已${judge}阈值${amount}FIL，请注意余额变化，以免影响业务运作`
const msgTemplateCodeBalance = "SMS_463925133"
const callTemplateBalance = `您的节点${minerid}的${item}已触发告警阈值，请注意余额变化，以免影响业务运作`
const callTemplateCodeBalance = "TTS_287100115"

func (br *BalanceRule) evaluate(ctx context.Context) (isAbnormal bool, msg []string, err error) {
	actor, err := global.Global.Adapter.Actor(ctx, chain.SmartAddress(*br.AccountAddr), nil)
	if err != nil {
		return false, nil, err
	}
	intPart := actor.Balance.Div(chain.AttoFilPrecision).IntPart()
	operand, err := strconv.Atoi(*br.Operand)
	if err != nil {
		return
	}

	switch *br.Operator {
	case "<=":
		if intPart <= int64(operand) {
			return true, []string{br.MinerOrAll, *br.AccountType, "小于等于"}, nil
		}
	case ">=":
		if intPart >= int64(operand) {
			return true, []string{br.MinerOrAll, *br.AccountType, "大于等于"}, nil
		}
	}
	return false, nil, nil
}

func (br *BalanceRule) Evaluate(ctx context.Context, ch chan<- *UuidExeRes) {
	defer func() {
		e := recover()
		if e != nil {
			logger.Errorln("something panic, rule uuid: ", br.UUID, e)
			debug.PrintStack()
		}
	}()
	logger.Infoln("组ID-", strconv.Itoa(int(br.GroupID)), br.MinerOrAll, br.GetDescription()+"【开始检查】")
	isAbnormal, msg, err := br.evaluate(ctx)
	if err != nil {
		return
	}
	ch <- &UuidExeRes{
		Uuid:       br.UUID,
		IsAbnormal: isAbnormal,
	}
	if isAbnormal {
		logger.Infoln("uuid-", br.UUID, " 组ID-", strconv.Itoa(int(br.GroupID)), br.MinerOrAll, br.GetDescription()+"【余额异常状态】") // 适配所有的默认分组 groupid 为0的情况
	}
	info := global.GetGlobalNotifyInfoMap(ctx, br.GetUUID()) // 存在问题：同一个miner下的节点可能在go协程中，如果多个相同uuid的不同actor触发异常，只进行一次
	if info == nil {
		return
	}
	if isAbnormal && time.Now().After(info.NotifyTime) {
		balanceSingleFlight.Do(br.GetUUID(), func() (interface{}, error) { //nolint
			logger.Warnln("组ID-", strconv.Itoa(int(br.GroupID)), br.MinerOrAll, br.GetDescription()+"【检查报警】\n接下来24h将不监测该规则")
			br.updateMemoryAlertInfo(ctx, br.GetUUID(), info)
			if info.NotifyCount < global.ContinuousNotifyDays {
				for _, notify := range br.AllNotify {
					go func(notify *NotifyReceiver) {
						receivers := notify.Receivers
						switch notify.NotifyType.GetDetail() {
						case "AlertMail":
							minerInfo := br.GetMinerInfo(ctx, msg[0])
							replaceMailMsg := br.ReplaceMailMsg(mailTemplateBalance, minerInfo)
							replaceMailMsg = strings.ReplaceAll(replaceMailMsg, "$description", br.Description)
							err = notify.NotifyType.Send(receivers, replaceMailMsg, []string{"节点余额告警"}) //发送参数，按照数组顺序
						case "AlertALiMsg":
							amount, err2 := strconv.Atoi(*br.Operand)
							if err2 != nil {
								logger.Infof(notify.NotifyType.GetDetail() + "strconv.Atoi 转换失败！")
							}
							str := fmt.Sprintf("{\"minerid\":\"%s\",\"item\":\"%s\",\"judge\":\"%s\",\"amount\":%d}", msg[0], msg[1]+"余额", msg[2], amount)
							err = notify.NotifyType.Send(receivers, msgTemplateCodeBalance, []string{str})
						case "AlertALiCall":
							str := fmt.Sprintf("{\"minerid\":\"%s\",\"item\":\"%s\"}", msg[0], *br.AccountType+"余额")
							err = notify.NotifyType.Send(receivers, callTemplateCodeBalance, []string{str}) //发送参数，按照数组顺序
						}
						if err != nil {
							logger.Infoln("组ID-", strconv.Itoa(int(br.GroupID)), br.MinerOrAll, br.GetDescription(), notify.NotifyType.GetDetail()+" 报警失败！", err)
						}
					}(notify)

				}
			}
			return nil, nil //上述发生错误的时候，只是进行日志打印，所以没返回err
		}) //nolint
	}
	logger.Infoln("组ID-", strconv.Itoa(int(br.GroupID)), br.MinerOrAll, br.GetDescription()+"【检查结束】")
}

func (br *BalanceRule) GetInterval() int64 {
	return br.Interval
}

func (br *BalanceRule) GetDescription() string {
	return br.Description
}

func (br *BalanceRule) GetIsActive() bool {
	return br.IsActive
}

func (br *BalanceRule) GetUpdateAt() time.Time {
	return br.UpdateAt
}
