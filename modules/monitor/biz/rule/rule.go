package rule

import (
	"context"
	"fmt"
	"strings"
	"time"

	logging "github.com/gozelle/logger"
	mapi "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/notify"
	mbo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/bo"
)

type UuidExeRes struct {
	Uuid       string
	IsAbnormal bool
}

type ActionOfRule interface {
	Evaluate(ctx context.Context, ch chan<- *UuidExeRes)
	GetInterval() int64
	GetDescription() string
	GetUUID() string
	GetIsActive() bool
	GetUpdateAt() time.Time
}

// 希望能给出一次监控所有规则执行的耗时

var logger = logging.NewLogger("rule")
var DitheringDuration = 30 * time.Minute

type BaseRule struct {
	UserID      int64             `json:"userID"`
	MType       mapi.MonitorType  `json:"monitor_type"`
	GroupID     int64             `json:"group_id"`
	MinerOrAll  string            `json:"miners"`
	UUID        string            `json:"uuid"`
	Operator    *string           `json:"operator"`
	Operand     *string           `json:"operand"`
	IsActive    bool              `json:"is_active"`
	Interval    int64             `json:"interval"`
	Description string            `json:"description"`
	UpdateAt    time.Time         `json:"update_at"`
	AllNotify   []*NotifyReceiver `json:"all_notify"`
}

func (br *BaseRule) GetUUID() string {
	return br.UUID
}

type NotifyReceiver struct {
	Receivers  []string
	NotifyType notify.AlertNotify //逻辑实现部分
}

func (br *BaseRule) GetEvaluateMiners(ctx context.Context) (miners []string) {
	if br.MinerOrAll != "" {
		miners = append(miners, br.MinerOrAll)
		return
	}
	// 建立group 与 miner的对应关系
	groupMinersMap := make(map[int64][]string)
	userMiners, err := global.Global.UserMinerRepo.SelectMinersByUserID(ctx, br.UserID) //获得加入组里的miner
	if err != nil {
		return
	}
	for _, miner := range userMiners {
		groupID := int64(0) // user_miners数据库中，group_id为null时候，表示为默认的分组
		if miner.GroupID != nil {
			groupID = *miner.GroupID
		}
		groupMinersMap[groupID] = append(groupMinersMap[groupID], miner.MinerID.Address())
	}
	// 如果指定了组的话，但没指定miner
	if br.GroupID != -1 && br.MinerOrAll == "" {
		miners = groupMinersMap[br.GroupID]
	}
	if br.GroupID == -1 && br.MinerOrAll == "" {
		for _, minerList := range groupMinersMap {
			miners = append(miners, minerList...)
		}
	}
	return miners
}

func (br *BaseRule) GetMinerInfo(ctx context.Context, miner string) (minerInfo *mbo.MinerInfo) {
	userMiners, err := global.Global.UserMinerRepo.SelectMinersByUserID(ctx, br.UserID) //获得加入组里的miner
	if err != nil {
		return
	}
	groups, err := global.Global.GroupRepo.SelectActiveGroupsByUserID(ctx, br.UserID)
	if err != nil {
		return
	}
	groupIDToNameMap := make(map[int64]string)
	for _, group := range groups {
		if group.GroupName == "default" {
			group.GroupName = "默认分组"
		}
		groupIDToNameMap[group.Id] = group.GroupName
	}
	for _, userMiner := range userMiners {
		if userMiner.MinerID.Address() == miner {
			groupID := int64(0)
			if userMiner.GroupID != nil {
				groupID = *userMiner.GroupID
			}
			return &mbo.MinerInfo{
				MinerID:   miner,
				MinerTag:  userMiner.MinerTag,
				GroupID:   groupID,
				GroupName: groupIDToNameMap[groupID],
			}
		}
	}
	return nil
}

func (br *BaseRule) ReplaceMailMsg(templateMsg string, minerInfo *mbo.MinerInfo) (msg string) {
	templateMsg = strings.ReplaceAll(templateMsg, "$group_name", minerInfo.GroupName)
	templateMsg = strings.ReplaceAll(templateMsg, "$miner_id", minerInfo.MinerID)
	if minerInfo.MinerTag != "" {
		templateMsg = strings.ReplaceAll(templateMsg, "$miner_tag", fmt.Sprintf("(%s)", minerInfo.MinerTag))
	} else {
		templateMsg = strings.ReplaceAll(templateMsg, "$miner_tag", "")
	}
	return templateMsg
}

func (br *BaseRule) updateMemoryAlertInfo(ctx context.Context, uuid string, info *global.NotifyInfo) {
	info.IsPreAbnormal = true
	info.NotifyCount++
	now := time.Now()
	info.AllowDitheringTime = now.Add(DitheringDuration)
	desiredTime := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	info.NotifyTime = desiredTime
	global.SetGlobalNotifyInfoMap(ctx, uuid, info)
}
