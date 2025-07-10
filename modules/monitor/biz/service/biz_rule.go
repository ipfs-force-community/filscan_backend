package mbiz

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/gozelle/mix"
	mapi "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/convertor"
	mbo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/bo"
	mdal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/dal"
	mpo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/po"
	mrepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/repo"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/vip"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

var GlobalRuleBiz *RuleBiz
var rc convertor.RuleConvertor

func NewRuleBiz(db *gorm.DB, adapter londobell.Adapter) *RuleBiz {
	GlobalRuleBiz = &RuleBiz{
		RuleRepo:  mdal.NewRuleDal(db),
		GroupRepo: prodal.NewGroupDal(db),
		DB:        db,
		Adapter:   adapter,
		Mutex:     new(sync.Mutex),
	}
	return GlobalRuleBiz
}

type RuleBiz struct {
	RuleRepo  mrepo.RuleRepo
	GroupRepo prorepo.GroupRepo
	DB        *gorm.DB
	Adapter   londobell.Adapter
	Mutex     *sync.Mutex
}

// SaveGroupPowerRules 单一职责：专门负责创建Power Rule
func SaveGroupPowerRules(ctx context.Context, groups []int64) (err error) {
	b := bearer.UseBearer(ctx)
	v := vip.UseVIP(ctx)
	var ruleList []*mpo.Rule
	for _, group := range groups {
		//第一次如果没有
		ruleUUID, err := GlobalRuleBiz.RuleRepo.SelectRuleUUID(ctx, b.Id, group, "", "PowerMonitor")
		if err != nil {
			return err
		}
		var alertUUIDStr string
		if ruleUUID != nil {
			alertUUIDStr = ruleUUID.Uuid
		} else {
			alertUUID, _ := uuid.NewUUID()
			alertUUIDStr = alertUUID.String()
		}
		isVip := false
		if v.MType != pro.NormalVIP {
			isVip = true
		}
		interval := mapi.PowerMonitorDefaultInterval
		ruleList = append(ruleList, &mpo.Rule{
			UserID:       b.Id,
			GroupID:      group,
			MinerIDOrAll: "",
			AccountType:  "",
			AccountAddr:  "",
			MonitorType:  "PowerMonitor",
			Uuid:         alertUUIDStr,
			MailAlert:    &b.Mail,
			Interval:     &interval,
			IsActive:     true,
			IsVip:        isVip,
			Description:  "1. 扇区发生错误 2. 扇区主动终止 3. 扇区正常到期",
		})
	}
	err = GlobalRuleBiz.RuleRepo.CreateUserRule(ctx, ruleList)
	return err
}

// SaveUserRules 方法只能被会员调用，所以不用考虑会员状态。可用于规则更新、更改告警方式、创建新的非Power规则
func (r *RuleBiz) SaveUserRules(ctx context.Context, req mapi.SaveUserRulesRequest) (resp *mapi.SaveUserRulesResponse, err error) {
	b := bearer.UseBearer(ctx)
	var ruleList []*mpo.Rule
	for _, item := range req.Items {
		// 判断请求添加组的规则是否属于该user，避免外界调用接口进行攻击
		if item.GroupIDOrAll != -1 && item.GroupIDOrAll != 0 {
			group, err := r.GroupRepo.SelectGroupByID(ctx, item.GroupIDOrAll)
			if err != nil {
				return nil, err
			}
			if group != nil && group.UserId != b.Id {
				err = mix.Codef(288, "Group: %s is not belong to your account!", group.GroupName)
				return nil, err
			}
		}
		var alertUUIDStr string
		if req.Update {
			// 适用于余额监控的修改情况，如果已经有节点拥有uuid，则从中拿取。或者仅仅是更新情况，不修改原来的uuid
			ruleUUID, err := r.RuleRepo.SelectRuleUUID(ctx, b.Id, item.GroupIDOrAll, item.MinerOrAll, item.MType.String())
			if err != nil {
				return resp, err
			}
			alertUUIDStr = ruleUUID.Uuid
		} else {
			alertUUID, _ := uuid.NewUUID()
			alertUUIDStr = alertUUID.String()
		}
		for _, rule := range item.Rules {
			// 判断类型所需要传输的参数是否都保证了，有无乱传参问题
			err := r.isValidRule(item.MType, rule)
			if err != nil {
				return resp, err
			}
			description := r.generateDesc(item.MType, rule)
			interval := r.generateInterval(item.MType, item.Interval)
			mailAlert := item.MailAlert
			if mailAlert != nil && *mailAlert != "" {
				receiver, err := rc.SplitReceiver(*mailAlert)
				if err != nil {
					return resp, err
				}
				var mails bytes.Buffer
				for _, recv := range receiver {
					if recv != b.Mail {
						mails.WriteString(recv + ",")
					}
				}
				mails.WriteString(b.Mail)
				tmpStr := mails.String()
				mailAlert = &tmpStr
			} else {
				tmpStr := b.Mail
				mailAlert = &tmpStr
			}
			if item.CallAlert != nil {
				str := removeDuplicate(*item.CallAlert)
				item.CallAlert = &str
			}
			if item.MsgAlert != nil {
				str := removeDuplicate(*item.MsgAlert)
				item.MsgAlert = &str
			}
			ruleList = append(ruleList, &mpo.Rule{
				UserID:       b.Id,
				GroupID:      item.GroupIDOrAll,
				MinerIDOrAll: item.MinerOrAll,
				AccountType:  rule.AccountType,
				AccountAddr:  rule.AccountAddr,
				MonitorType:  string(item.MType),
				Uuid:         alertUUIDStr,
				Operator:     rule.Operator,
				Operand:      rule.Operand,
				CallAlert:    item.CallAlert,
				MsgAlert:     item.MsgAlert,
				MailAlert:    mailAlert,
				Interval:     &interval,
				IsActive:     true,
				IsVip:        true,
				Description:  description,
			})
		}
	}
	err = r.RuleRepo.CreateUserRule(ctx, ruleList)
	if err != nil {
		return nil, err
	}
	var items []*mapi.SaveUserRulesResp
	// 有时间再抽取出来
	for _, rule := range ruleList {
		items = append(items, &mapi.SaveUserRulesResp{
			MType:        mapi.MonitorType(rule.MonitorType),
			GroupIDOrAll: rule.GroupID,
			MinerOrAll:   rule.MinerIDOrAll,
			Operator:     rule.Operator,
			Operand:      rule.Operand,
			MailAlert:    rule.MailAlert,
			MsgAlert:     rule.MsgAlert,
			CallAlert:    rule.CallAlert,
			Interval:     rule.Interval,
			IsActive:     true,
			Description:  rule.Description,
		})
	}
	resp = &mapi.SaveUserRulesResponse{Items: items}
	return resp, nil
}

func (r *RuleBiz) generateInterval(mType mapi.MonitorType, i *int64) int64 {
	var interval int64
	if i != nil {
		interval = *i
	} else {
		switch mType {
		case "PowerMonitor":
			interval = mapi.PowerMonitorDefaultInterval
		case "BalanceMonitor":
			interval = mapi.BalanceMonitorDefaultInterval
		case "ExpireSectorMonitor":
			interval = mapi.ExpireSectorMonitorDefaultInterval
		}
	}
	return interval
}

func (r *RuleBiz) generateDesc(mType mapi.MonitorType, rule mapi.SingleRule) string {
	res := "1. 扇区发生错误 2. 扇区主动终止 3. 扇区正常到期"
	category := ""
	operator := "小于等于"
	unit := ""
	if mType != "PowerMonitor" {
		switch mType {
		case "BalanceMonitor":
			if rule.AccountType != "" {
				category = rule.AccountType + "余额"
			}
			unit = "FIL"
		case "ExpireSectorMonitor":
			category = "扇区到期时间"
			unit = "天"
		}
	} else {
		return res
	}
	if rule.Operator != nil && rule.Operand != nil {
		switch *rule.Operator {
		case "<=":
			operator = " 小于等于 "
		case ">=":
			operator = " 大于等于 "
		}
		res = category + operator + *rule.Operand + unit
	}
	return res
}

func (r *RuleBiz) DeleteUserRule(ctx context.Context, req mapi.DeleteUserRulesRequest) (resp *mapi.DeleteUserRulesResponse, err error) {
	b := bearer.UseBearer(ctx)
	affected, err := r.RuleRepo.DeleteUUIDRule(ctx, b.Id, req.Uuid)
	if err != nil {
		return resp, err
	}
	resp = &mapi.DeleteUserRulesResponse{IsDelete: true}
	if affected == 0 {
		resp.IsDelete = false
	}
	return resp, nil
}

func (r *RuleBiz) GetUserRules(ctx context.Context, req mapi.GetUserRulesRequest) (resp mapi.GetUserRulesResponse, err error) {
	b := bearer.UseBearer(ctx)
	userRules, err := r.RuleRepo.SelectRulesByUserIDAndType(ctx, b.Id, string(req.MType)) //返回带uuid的
	if err != nil {
		return
	}
	minerTagMap, tagMinerMap, err := r.getMinerTagMap(ctx, b.Id)
	if err != nil {
		return
	}
	var tagToMinerID []string
	if req.MinerTag != "" {
		if req.MinerOrAll != "" && minerTagMap[req.MinerOrAll] != req.MinerTag {
			return resp, fmt.Errorf("miner tag does not match miner id")
		}
		tagToMinerID = tagMinerMap[req.MinerTag]
	}
	// 将相同的groupid、minerid、创建时间的规则进行聚合
	//timeRulesMap := make(map[int64][]*mpo.Rule)
	//for _, rule := range userRules {
	//	timeRulesMap[rule.CreatedAt.Unix()] = append(timeRulesMap[rule.CreatedAt.Unix()], rule)
	//}
	//// 在同一个时间为同一次建立的规则，再对这些规则按是否为同一个groupid、minerid判断
	//// 为后续修改增加判断条件
	//for _, rules := range timeRulesMap {
	//	groupMinerMap := make(map[int64]map[string][]*mpo.Rule)
	//	var groupId int64 = 0
	//	minerId := ""
	//	for _, rule := range rules { //聚合
	//		// 同一时间
	//		if rule.GroupID != nil {
	//			groupId = *rule.GroupID
	//		}
	//		if rule.MinerIDOrAll != nil {
	//			minerId = *rule.MinerIDOrAll
	//		}
	//		if groupMinerMap[groupId] == nil {
	//			groupMinerMap[groupId] = make(map[string][]*mpo.Rule)
	//		}
	//		groupMinerMap[groupId][minerId] = append(groupMinerMap[groupId][minerId], rule)
	//
	//	}
	//	for _, minerMap := range groupMinerMap { //看上去三重循环，实际时间复杂度才总规则数
	//		for _, rl := range minerMap {
	//			toGetUserRules := r.convertToGetUserRules(rl)
	//			resp.Items = append(resp.Items, toGetUserRules)
	//		}
	//	}
	//}
	// 根据group和miner来判断显示所有还是特定类型
	tempUUIDMap := make(map[string][]*mpo.Rule)
	UUIDMap := make(map[string][]*mpo.Rule)
	for _, rule := range userRules {
		tempUUIDMap[rule.Uuid] = append(tempUUIDMap[rule.Uuid], rule)
	}
	// 如果有确定的group和miner则直接对应，无则所有
	for _, rules := range tempUUIDMap { //中间过程筛选，根据规则过滤想要分组或节点的结果，确定最终的uuid - rules map
		//1.选择全部规则的状况
		if req.GroupIDOrAll == -1 && req.MinerOrAll == "" && req.MinerTag == "" {
			UUIDMap[rules[0].Uuid] = rules
		} else if req.MinerTag == "" && req.MinerOrAll == "" && rules[0].GroupID == req.GroupIDOrAll { //3.按照某一列去选择，如仅分组或仅标签或仅miner
			UUIDMap[rules[0].Uuid] = rules
		} else if req.MinerOrAll != "" && rules[0].MinerIDOrAll == req.MinerOrAll {
			// 要是只指定了minerid，此时Groupid = -1，可只考虑minerid这一项
			if req.GroupIDOrAll == -1 {
				UUIDMap[rules[0].Uuid] = rules
			} else if req.GroupIDOrAll == rules[0].GroupID {
				UUIDMap[rules[0].Uuid] = rules
			}
			// 主要用于有些规则保存时候组可能为-1，但是查找的时候要从组、miner来查询，这时候只需要判断miner就行，有miner时候只考虑miner，有tag的时候转换成miner考虑miner
			//UUIDMap[rules[0].Uuid] = rules
		} else if req.MinerTag != "" && checkInSlice(rules[0].MinerIDOrAll, tagToMinerID) {
			UUIDMap[rules[0].Uuid] = rules
		}
		//else if req.GroupIDOrAll >= 0 && rules[0].GroupID == req.GroupIDOrAll && rules[0].MinerIDOrAll == req.MinerOrAll { // 2.选择某个分组下的某个特定的miner
		//	UUIDMap[rules[0].Uuid] = rules
		//} else if req.MinerTag == "" && req.GroupIDOrAll == -1 && rules[0].MinerIDOrAll == req.MinerOrAll {
		//	UUIDMap[rules[0].Uuid] = rules
		//} else if req.MinerTag != "" && req.MinerOrAll == "" && req.GroupIDOrAll == -1 && tagToMinerID == rules[0].MinerIDOrAll {
		//	UUIDMap[rules[0].Uuid] = rules
		//}
		//if req.MinerTag != "" {
		//	if rules[0].MinerIDOrAll == req.MinerOrAll {
		//		UUIDMap[rules[0].Uuid] = rules
		//	}
		//} else if req.GroupIDOrAll >= 0 {
		//	//即指定了某规则
		//	if rules[0].GroupID == req.GroupIDOrAll && rules[0].MinerIDOrAll == req.MinerOrAll {
		//		UUIDMap[rules[0].Uuid] = rules
		//	}
		//} else {
		//	UUIDMap[rules[0].Uuid] = rules
		//}
	}
	for uuidValue, rules := range UUIDMap { //中间过程筛选，根据规则过滤想要分组或节点的结果，确定最终的uuid - rules map
		toGetUserRules := r.convertToGetUserRules(rules)
		toGetUserRules.UUID = uuidValue
		if toGetUserRules.GroupID == 0 {
			toGetUserRules.GroupName = "default_group"
			toGetUserRules.IsDefault = true
		} else if toGetUserRules.GroupID != -1 {
			groupInfo, err := r.GroupRepo.SelectGroupByID(ctx, toGetUserRules.GroupID)
			if err != nil {
				return resp, err
			}
			toGetUserRules.GroupName = groupInfo.GroupName
		} else {
			toGetUserRules.GroupName = "ALL"
		}
		if toGetUserRules.MinerIDOrAll != "" {
			toGetUserRules.MinerTag = minerTagMap[toGetUserRules.MinerIDOrAll]
		}
		resp.Items = append(resp.Items, toGetUserRules)
	}
	//最终结果进行排序
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].CreatedAt.After(resp.Items[j].CreatedAt)
	})
	return
}

func checkInSlice(miner string, miners []string) bool {
	for _, m := range miners {
		if miner == m {
			return true
		}
	}
	return false
}

func (r *RuleBiz) convertToGetUserRules(rules []*mpo.Rule) (getUserRules *mapi.GetUserRules) {
	getUserRules = &mapi.GetUserRules{}
	getUserRules.UserID = rules[0].UserID
	getUserRules.GroupID = rules[0].GroupID
	getUserRules.MinerIDOrAll = rules[0].MinerIDOrAll
	getUserRules.MonitorType = rules[0].MonitorType
	getUserRules.CreatedAt = rules[0].CreatedAt
	getUserRules.IsActive = rules[0].IsActive
	ruleConvertor := convertor.RuleConvertor{}
	if rules[0].CallAlert != nil {
		receiver, err := ruleConvertor.SplitReceiver(*rules[0].CallAlert)
		if err != nil {
			return
		}
		getUserRules.CallAlert = append(getUserRules.CallAlert, receiver...)
	}
	if rules[0].MsgAlert != nil {
		receiver, err := ruleConvertor.SplitReceiver(*rules[0].MsgAlert)
		if err != nil {
			return
		}
		getUserRules.MsgAlert = append(getUserRules.MsgAlert, receiver...)
	}
	if rules[0].MailAlert != nil {
		receiver, err := ruleConvertor.SplitReceiver(*rules[0].MailAlert)
		if err != nil {
			return
		}
		getUserRules.MailAlert = append(getUserRules.MailAlert, receiver...)
	}
	for _, rule := range rules {
		getUserRules.Description = append(getUserRules.Description, rule.Description)
		getUserRules.Rules = append(getUserRules.Rules, &mapi.SingleRule{
			AccountType: rule.AccountType,
			AccountAddr: rule.AccountAddr,
			Operator:    rule.Operator,
			Operand:     rule.Operand,
		})
	}
	return getUserRules
}

func (r *RuleBiz) UpdateRuleActiveState(ctx context.Context, req mapi.UpdateActiveStateRequest) (resp mapi.UpdateActiveStateResponse, err error) {
	b := bearer.UseBearer(ctx)
	rowsAffected, err := r.RuleRepo.UpdateActiveState(ctx, b.Id, req.Uuid)
	if err != nil || rowsAffected == 0 {
		return
	}
	rules, err := r.RuleRepo.SelectRulesByUUID(ctx, b.Id, req.Uuid)
	if err != nil {
		return
	}
	toGetUserRules := r.convertToGetUserRules(rules)
	toGetUserRules.UUID = req.Uuid
	resp.UpdateActiveState = toGetUserRules
	return
}

func (r *RuleBiz) getMinerTagMap(ctx context.Context, userID int64) (minerTagMap map[string]string, tagMinerMap map[string][]string, err error) {
	minerTagMap = make(map[string]string)
	tagMinerMap = make(map[string][]string)
	minerTags, err := r.RuleRepo.SelectMinersByUserID(ctx, userID)
	if err != nil {
		return
	}
	for _, miner := range minerTags {
		minerTagMap[miner.MinerID.Address()] = miner.MinerTag
		tagMinerMap[miner.MinerTag] = append(tagMinerMap[miner.MinerTag], miner.MinerID.Address())
	}

	return minerTagMap, tagMinerMap, nil
}

func (r *RuleBiz) GetRuleMinerInfo(ctx context.Context, req mapi.GetMinerInfoRequest) (resp mapi.GetMinerInfoResponse, err error) {
	detail, err := r.Adapter.Miner(ctx, chain.SmartAddress(req.MinerID), nil)
	if err != nil {
		return
	}
	minerDetail, err := r.convertMinerDetail(ctx, detail)
	if err != nil {
		return
	}
	resp.MinerDetail = minerDetail
	return resp, nil
}

func (r *RuleBiz) convertMinerDetail(ctx context.Context, detail *londobell.MinerDetail) (minerDetail *mbo.MinerDetail, err error) {
	minerDetail = &mbo.MinerDetail{
		Epoch: detail.Epoch,
	}
	balance, err := r.getMinerBalance(ctx, detail.Miner)
	if err != nil {
		return
	}
	minerDetail.Accounts = append(minerDetail.Accounts, mbo.AccountAddrBalance{Type: "Miner", Addr: detail.Miner, Balance: balance})
	balance, err = r.getMinerBalance(ctx, detail.Owner)
	if err != nil {
		return
	}
	minerDetail.Accounts = append(minerDetail.Accounts, mbo.AccountAddrBalance{Type: "Owner", Addr: detail.Owner, Balance: balance})
	balance, err = r.getMinerBalance(ctx, detail.Worker)
	if err != nil {
		return
	}
	minerDetail.Accounts = append(minerDetail.Accounts, mbo.AccountAddrBalance{Type: "Worker", Addr: detail.Worker, Balance: balance})
	balance, err = r.getMinerBalance(ctx, detail.Beneficiary)
	if err != nil {
		return
	}
	minerDetail.Accounts = append(minerDetail.Accounts, mbo.AccountAddrBalance{Type: "Beneficiary", Addr: detail.Beneficiary, Balance: balance})
	for i, controller := range detail.Controllers {
		balance, err = r.getMinerBalance(ctx, controller)
		if err != nil {
			return
		}
		str := "Controller_" + strconv.Itoa(i)
		minerDetail.Accounts = append(minerDetail.Accounts, mbo.AccountAddrBalance{Type: str, Addr: controller, Balance: balance})
	}
	return
}

func (r *RuleBiz) isValidRule(mType mapi.MonitorType, rule mapi.SingleRule) (err error) {
	switch mType {
	case "PowerMonitor":
		if rule.AccountAddr != "" || rule.AccountType != "" || rule.Operator != nil || rule.Operand != nil {
			return fmt.Errorf("too many parameters are given")
		}
	case "BalanceMonitor":
		if rule.Operator == nil || rule.Operand == nil || rule.AccountAddr == "" || rule.AccountType == "" {
			return fmt.Errorf("not enough parameters are given")
		}
	case "ExpireSectorMonitor":
		if rule.Operator == nil || rule.Operand == nil {
			return fmt.Errorf("not enough parameters are given")
		}
		if rule.AccountAddr != "" || rule.AccountType != "" {
			return fmt.Errorf("too many parameters are given")
		}
	}
	return nil
}

func (r *RuleBiz) getMinerBalance(ctx context.Context, minerID string) (balance string, err error) {
	detail, err := r.Adapter.Actor(ctx, chain.SmartAddress(minerID), nil)
	if err != nil {
		return
	}
	return detail.Balance.String(), nil
}

func removeDuplicate(receiversStr string) (adjReceiversStr string) {
	receivers, err := rc.SplitReceiver(receiversStr)
	if err != nil {
		return
	}
	m := make(map[string]struct{})
	var buffer bytes.Buffer
	for i, receiver := range receivers {
		if _, ok := m[receiver]; !ok {
			m[receiver] = struct{}{}
			if i != 0 {
				buffer.WriteString("," + receiver)
			} else {
				buffer.WriteString(receiver)
			}
		}
	}
	return buffer.String()
}
