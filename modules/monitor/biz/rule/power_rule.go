package rule

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type PowerRule struct {
	BaseRule
}

const mailTemplatePower = `您好，您的节点$group_name $miner_id$miner_tag 由于发生$error_type致使算力掉落，请您及时查看，以免影响业务运作。`
const msgTemplatePower = `您的节点${minerid}由于发生${errortype}致使算力掉落 ，请及时查看，以免影响业务运作`
const msgTemplateCodePower = "SMS_463895156"
const callTemplatePower = `您的节点${minerid}由于发生${errortype}致使算力掉落，请及时查看，以免影响业务运作`
const callTemplateCodePower = "TTS_287805120"

func (pr *PowerRule) Evaluate(ctx context.Context, ch chan<- *UuidExeRes) {
	defer func() {
		e := recover()
		if e != nil {
			logger.Errorln("something panic, rule uuid: ", pr.UUID)
			debug.PrintStack()
		}
	}()
	logger.Infoln("组ID-", strconv.Itoa(int(pr.GroupID)), pr.GetUUID()+"算力【开始检查】")
	isAbnormal, msg, err := pr.evaluate(ctx)
	if err != nil {
		return
	}
	ch <- &UuidExeRes{
		Uuid:       pr.UUID,
		IsAbnormal: isAbnormal,
	}
	info := global.GetGlobalNotifyInfoMap(ctx, pr.GetUUID())
	if info == nil {
		return
	}
	if isAbnormal && time.Now().After(info.NotifyTime) {
		logger.Warnln("组ID-", strconv.Itoa(int(pr.GroupID)), pr.GetUUID()+"算力【检查报警】\n接下来24h将不监测该规则")
		pr.updateMemoryAlertInfo(ctx, pr.GetUUID(), info)
		if info.NotifyCount < global.ContinuousNotifyDays {
			for _, notify := range pr.AllNotify {
				go func(notify *NotifyReceiver) {
					receivers := notify.Receivers
					switch notify.NotifyType.GetDetail() {
					case "AlertMail":
						minerInfo := pr.GetMinerInfo(ctx, msg[0])
						replaceMailMsg := pr.ReplaceMailMsg(mailTemplatePower, minerInfo)
						replaceMailMsg = strings.ReplaceAll(replaceMailMsg, "$error_type", msg[1])
						err = notify.NotifyType.Send(receivers, replaceMailMsg, []string{"节点算力告警"}) //发送参数，按照数组顺序
					case "AlertALiMsg":
						str := fmt.Sprintf("{\"minerid\":\"%s\",\"errortype\":\"%s\"}", msg[0], msg[1])
						err = notify.NotifyType.Send(receivers, msgTemplateCodePower, []string{str})
					case "AlertALiCall":
						str := fmt.Sprintf("{\"minerid\":\"%s\",\"errortype\":\"%s\"}", msg[0], msg[1])
						err = notify.NotifyType.Send(receivers, callTemplateCodePower, []string{str}) //发送参数，按照数组顺序
					}
					if err != nil {
						logger.Infoln("组ID-", strconv.Itoa(int(pr.GroupID)), pr.GetUUID(), notify.NotifyType.GetDetail()+" 报警失败！", err)
					}
				}(notify)
			}
		}
	}
	logger.Infoln("组ID-", strconv.Itoa(int(pr.GroupID)), pr.GetUUID()+"算力【检查结束】")
}

func (pr *PowerRule) evaluate(ctx context.Context) (isAbnormal bool, msg []string, err error) {
	now := chain.CalcEpochByTime(time.Now()) - 1 // tolerant
	detectiveEpoch := now - 120
	evaluateMiners := pr.GetEvaluateMiners(ctx)
	for _, evaluateMiner := range evaluateMiners {
		miner, err := global.Global.Adapter.Miner(ctx, chain.SmartAddress(evaluateMiner), &now)
		if err != nil {
			return false, nil, err
		}

		minerLastState, err := global.Global.Adapter.Miner(ctx, chain.SmartAddress(evaluateMiner), &detectiveEpoch)
		if err != nil {
			return false, nil, err
		}

		if miner.FaultSectorCount > minerLastState.FaultSectorCount {
			return true, []string{miner.Miner, "扇区错误"}, nil
		}

		if miner.TerminateSectorCount > minerLastState.TerminateSectorCount {
			return true, []string{miner.Miner, "扇区主动终止/扇区正常到期"}, nil
		} //只需要考虑两种错误即可，主动终止和正常到期看起来差不太多
	}

	return false, nil, nil
}

func (pr *PowerRule) GetInterval() int64 {
	return pr.Interval
}

func (pr *PowerRule) GetDescription() string {
	return pr.Description
}

func (pr *PowerRule) GetIsActive() bool {
	return pr.IsActive
}

func (pr *PowerRule) GetUpdateAt() time.Time {
	return pr.UpdateAt
}
