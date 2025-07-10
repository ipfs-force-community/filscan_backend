package convertor

import (
	"strings"

	mapi "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/notify"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/rule"
	mpo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/po"
)

type RuleConvertor struct {
}

// 切割一下所有的邮箱、电话
func (rc RuleConvertor) SplitReceiver(receiversStr string) ([]string, error) {
	var receivers []string
	for _, receiver := range strings.Split(receiversStr, ",") {
		if receiver != "" {
			receivers = append(receivers, strings.TrimSpace(receiver))
		}
	}
	return receivers, nil
}

func (rc RuleConvertor) GetAllNotifyReceiver(source *mpo.Rule) (res []*rule.NotifyReceiver, err error) {
	if source.CallAlert != nil {
		callReceivers, err := rc.SplitReceiver(*source.CallAlert)
		if err != nil {
			return nil, err
		}
		if callReceivers != nil {
			res = append(res, &rule.NotifyReceiver{
				Receivers:  callReceivers,
				NotifyType: &notify.GlobalNotify.AlertALiCall,
			})
		}
	}
	if source.MsgAlert != nil {
		msgReceivers, err := rc.SplitReceiver(*source.MsgAlert)
		if err != nil {
			return nil, err
		}
		if msgReceivers != nil {
			res = append(res, &rule.NotifyReceiver{
				Receivers:  msgReceivers,
				NotifyType: &notify.GlobalNotify.AlertALiMsg,
			})
		}
	}
	if source.MailAlert != nil {
		mailReceivers, err := rc.SplitReceiver(*source.MailAlert)
		if err != nil {
			return nil, err
		}
		if mailReceivers != nil {
			res = append(res, &rule.NotifyReceiver{
				Receivers:  mailReceivers,
				NotifyType: &notify.GlobalNotify.AlertMail,
			})
		}
	}
	return res, nil
}

// 根据类型转为接口
func (rc RuleConvertor) ToRule(source *mpo.Rule) (target rule.ActionOfRule) {
	baseRule := rule.BaseRule{
		UserID:      source.UserID,
		MType:       mapi.MonitorType(source.MonitorType),
		GroupID:     source.GroupID,
		MinerOrAll:  source.MinerIDOrAll,
		Operator:    source.Operator,
		Operand:     source.Operand,
		IsActive:    source.IsActive,
		Interval:    *source.Interval,
		UUID:        source.Uuid,
		Description: source.Description,
		UpdateAt:    source.UpdatedAt,
	}
	allNotify, err := rc.GetAllNotifyReceiver(source)
	if err != nil {
		return
	}
	baseRule.AllNotify = allNotify
	switch source.MonitorType {
	case "PowerMonitor":
		target = &rule.PowerRule{BaseRule: baseRule}
	case "BalanceMonitor":
		target = &rule.BalanceRule{BaseRule: baseRule, AccountType: &source.AccountType, AccountAddr: &source.AccountAddr}
	case "ExpireSectorMonitor":
		target = &rule.ExpireSectorRule{BaseRule: baseRule}
	}
	return
}

func (rc RuleConvertor) ToRuleList(source []*mpo.Rule) (target []rule.ActionOfRule) {
	for _, r := range source {
		target = append(target, rc.ToRule(r))
	}
	return
}
