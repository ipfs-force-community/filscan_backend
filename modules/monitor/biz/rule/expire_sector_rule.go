package rule

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type ExpireSectorRule struct {
	BaseRule
	AccountAddr string
}

const mailTemplateExpireSector = `您好，您的节点$group_name $miner_id$miner_tag 部分$description，请及时续期，以免影响业务运作。`
const msgTemplateExpireSector = `您的节点${minerid}部分扇区到期时间已小于等于${day}天，请及时续期，以免影响业务运作`
const msgTemplateCodeExpireSector = "SMS_463594957"
const callTemplateExpireSector = `您的节点${minerid}部分扇区到期时间已小于等于${day}天，请及时续期，以免影响业务运作`
const callTemplateCodeExpireSector = "TTS_287070112"

func (er *ExpireSectorRule) Evaluate(ctx context.Context, ch chan<- *UuidExeRes) {
	defer func() {
		e := recover()
		if e != nil {
			logger.Errorln("something panic, rule uuid: ", er.UUID)
			debug.PrintStack()
		}
	}()
	logger.Infoln("组ID-", strconv.Itoa(int(er.GroupID)), er.MinerOrAll, er.GetDescription()+"【开始检查】")
	isAbnormal, msg, err := er.evaluate(ctx)
	if err != nil {
		return
	}
	ch <- &UuidExeRes{
		Uuid:       er.UUID,
		IsAbnormal: isAbnormal,
	}
	info := global.GetGlobalNotifyInfoMap(ctx, er.GetUUID())
	if info == nil {
		return
	}
	if isAbnormal && time.Now().After(info.NotifyTime) {
		logger.Warnln("组ID-", strconv.Itoa(int(er.GroupID)), er.MinerOrAll, er.GetDescription()+"【检查报警】\n接下来24h将不监测该规则")
		er.updateMemoryAlertInfo(ctx, er.GetUUID(), info)
		if info.NotifyCount < global.ContinuousNotifyDays { //超过了限制时间后就不发了
			for _, notify := range er.AllNotify {
				go func(notify *NotifyReceiver) {
					receivers := notify.Receivers
					switch notify.NotifyType.GetDetail() {
					case "AlertMail":
						minerInfo := er.GetMinerInfo(ctx, msg[0])
						replaceMailMsg := er.ReplaceMailMsg(mailTemplateExpireSector, minerInfo)
						replaceMailMsg = strings.ReplaceAll(replaceMailMsg, "$description", er.Description)
						err = notify.NotifyType.Send(receivers, replaceMailMsg, []string{"节点扇区到期告警"}) //发送参数，按照数组顺序
					case "AlertALiMsg":
						str := fmt.Sprintf("{\"minerid\":\"%s\",\"day\":\"%s\"}", msg[0], msg[1])
						err = notify.NotifyType.Send(receivers, msgTemplateCodeExpireSector, []string{str}) //发送参数，按照数组顺序
					case "AlertALiCall":
						str := fmt.Sprintf("{\"minerid\":\"%s\",\"day\":\"%s\"}", msg[0], msg[1])
						err = notify.NotifyType.Send(receivers, callTemplateCodeExpireSector, []string{str}) //发送参数，按照数组顺序
					}
					if err != nil {
						logger.Infoln("组ID-", strconv.Itoa(int(er.GroupID)), er.MinerOrAll, notify.NotifyType.GetDetail()+" 报警失败！", err)
					}
				}(notify)
			}
		}
	}
	logger.Infoln("组ID-", strconv.Itoa(int(er.GroupID)), er.MinerOrAll, er.GetDescription()+"【检查结束】")
}

func (er *ExpireSectorRule) evaluate(ctx context.Context) (valid bool, errorMsg []string, err error) {
	epoch := int64(0)
	err = global.Global.DB.Raw(`select max(epoch) from pro.miner_sectors where epoch <= (select epoch from chain.sync_syncers where name = ?)`, syncer.SectorSyncer).Scan(&epoch).Error
	if err != nil {
		return false, nil, err
	}
	evaluateMiners := er.GetEvaluateMiners(ctx)
	sectors, err := global.Global.MinerRepo.GetMinersSectors(ctx, epoch, evaluateMiners)
	if err != nil {
		return false, nil, err
	}
	day, ok := math.ParseUint64(*er.Operand)
	if !ok {
		return false, nil, fmt.Errorf("parse operand failed, data is not a valid uint")
	}
	endEpoch := chain.CalcEpochByTime(chain.CurrentEpoch().Time().Add(time.Hour * 24 * time.Duration(day)))
	for i := range sectors {
		if chain.Epoch(sectors[i].Epoch) <= endEpoch {
			return true, []string{sectors[i].Miner, *er.Operand}, nil
		}
	}

	return false, nil, nil
}

func (er *ExpireSectorRule) GetInterval() int64 {
	return er.Interval
}

func (er *ExpireSectorRule) GetDescription() string {
	return er.Description
}

func (er *ExpireSectorRule) GetIsActive() bool {
	return er.IsActive
}

func (er *ExpireSectorRule) GetUpdateAt() time.Time {
	return er.UpdateAt
}
