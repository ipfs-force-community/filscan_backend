package mapi

import (
	"context"
	"time"

	mbo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/bo"
)

// 这些还是要注册到节点管家里面去的pro，因为要鉴权
type RuleManager interface {
	// 返回的应该是个数组，多个rule
	GetUserRules(ctx context.Context, req GetUserRulesRequest) (resp GetUserRulesResponse, err error) //获取所有、或者对应的类型type的规则
	DeleteUserRule(ctx context.Context, req DeleteUserRulesRequest) (resp *DeleteUserRulesResponse, err error)
	SaveUserRules(ctx context.Context, req SaveUserRulesRequest) (resp *SaveUserRulesResponse, err error)
	UpdateRuleActiveState(ctx context.Context, req UpdateActiveStateRequest) (resp UpdateActiveStateResponse, err error)
	GetRuleMinerInfo(ctx context.Context, req GetMinerInfoRequest) (resp GetMinerInfoResponse, err error)
}

type RuleEvaluate interface{}

type MonitorType string

func (m MonitorType) String() string {
	return string(m)
}

const (
	BalanceMonitor      MonitorType = "BalanceMonitor"
	ExpireSectorMonitor MonitorType = "ExpireSectorMonitor"
	PowerMonitor        MonitorType = "PowerMonitor"
)

const (
	BalanceMonitorDefaultInterval      int64 = 15
	ExpireSectorMonitorDefaultInterval int64 = 15
	PowerMonitorDefaultInterval        int64 = 15
)

type GetUserRulesRequest struct {
	MType        MonitorType `json:"monitor_type"`
	GroupIDOrAll int64       `json:"group_id_or_all"` //-1 表示 all
	MinerOrAll   string      `json:"miner_or_all"`    //在前面获取了标签与miner节点的对应
	MinerTag     string      `json:"miner_tag"`
}

// GetUserRules 这里会对balance规则下多种不同类型账户的规则做一个聚合
type GetUserRules struct {
	UserID       int64         `json:"user_id"`
	GroupID      int64         `json:"group_id"`
	IsDefault    bool          `json:"is_default"`
	GroupName    string        `json:"group_name"`
	MinerIDOrAll string        `json:"miner_id_or_all"`
	MinerTag     string        `json:"miner_tag"`
	MonitorType  string        `json:"monitor_type"`
	UUID         string        `json:"uuid"`
	Rules        []*SingleRule `json:"rules"`
	MailAlert    []string      `json:"mail_alert"`
	MsgAlert     []string      `json:"msg_alert,omitempty"`
	CallAlert    []string      `json:"call_alert,omitempty"`
	IsActive     bool          `json:"is_active"`
	Description  []string      `json:"description"`
	CreatedAt    time.Time     `json:"created_at"`
}

type GetUserRulesResponse struct {
	Items []*GetUserRules `json:"items,omitempty"`
}

type DeleteUserRulesRequest struct {
	Uuid string `json:"uuid"`
}

type DeleteUserRulesResponse struct {
	IsDelete bool `json:"is_delete"`
}

type UpdateActiveStateRequest struct {
	Uuid string `json:"uuid,omitempty"`
}

type UpdateActiveStateResponse struct {
	//返回更新过的该数据啦 根据uuid
	UpdateActiveState *GetUserRules `json:"update_active_state"`
}

type GetMinerInfoRequest struct {
	MinerID string `json:"miner_id,omitempty"`
}

type GetMinerInfoResponse struct {
	//返回更新过的该数据啦 根据uuid
	MinerDetail *mbo.MinerDetail `json:"miner_detail,omitempty"`
}

type SaveUserRulesRequest struct {
	Items  []*SaveUserRulesReq `json:"items"`
	Update bool                `json:"update"`
}

type SaveUserRulesReq struct {
	MType        MonitorType  `json:"monitor_type"`
	GroupIDOrAll int64        `json:"group_id_or_all"` //-1 表示 all
	MinerOrAll   string       `json:"miner_or_all"`
	Rules        []SingleRule `json:"rules"`
	MailAlert    *string      `json:"mail_alert,omitempty"`
	MsgAlert     *string      `json:"msg_alert,omitempty"`
	CallAlert    *string      `json:"call_alert,omitempty"`
	Interval     *int64       `json:"interval,omitempty"` //todo 保留给未来自定义间隔接口
}

type SingleRule struct {
	AccountType string  `json:"account_type"`
	AccountAddr string  `json:"account_addr"`
	Operator    *string `json:"operator"`
	Operand     *string `json:"operand"`
}

type SaveUserRulesResponse struct {
	Items []*SaveUserRulesResp `json:"items,omitempty"`
}

// SaveUserRulesResp 其实操作数和操作符也不是必须的，因为有些规则是定好的，只需要指定分组
type SaveUserRulesResp struct {
	MType        MonitorType `json:"monitor_type"`
	GroupIDOrAll int64       `json:"group_id_or_all"` //-1 表示 all
	MinerOrAll   string      `json:"miner_or_all"`
	Operator     *string     `json:"operator"`
	Operand      *string     `json:"operand"`
	MailAlert    *string     `json:"mail_alert,omitempty"`
	MsgAlert     *string     `json:"msg_alert,omitempty"`
	CallAlert    *string     `json:"call_alert,omitempty"`
	Interval     *int64      `json:"interval,omitempty"`
	IsActive     bool        `json:"is_active"`
	Description  string      `json:"description"`
}
