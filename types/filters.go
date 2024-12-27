package types

import (
	"math/big"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type Filters struct {
	Index       int64         `json:"index"`        // 页码数
	Limit       int64         `json:"limit"`        // 每页显示数量
	Start       *chain.Epoch  `json:"start"`        // 起始高度
	End         *chain.Epoch  `json:"end"`          // 结束高度
	Interval    *IntervalType `json:"interval"`     // 时间间隔：24h，7d，30d, 365d
	MethodName  string        `json:"method_name"`  // 消息类型名称
	AccountType *AccountType  `json:"account_type"` // 账户类型
	InputType   *InputType    `json:"input_type"`   // 搜索框输入类型
	PageSize    *big.Int      `json:"page_size"`    // 区块页显示列表长度
}

// 两套逻辑请求:支持cid与高度搜索
// 1: cid + len cid作为end高度,cid - len作为start高度
// 2: (start,end]
type TipsetFilters struct {
	Cid    string `json:"cid"`   // 待查询的cid
	Length int64  `json:"len"`   // 返回列表长度,与cid绑定
	Start  int64  `json:"start"` // 起始高度
	End    int64  `json:"end"`   // 结束高度
}

const (
	ACCOUNT     = "account"
	MINER       = "miner"
	OWNER       = "owner"
	MULTISIG    = "multisig"
	EVM         = "evm"
	ETHACCOUNT  = "ethaccount"
	PLACEHOLDER = "placeholder"
	EMPTY       = "<empty>"
	TOBECREATED = "To be created"
	DAY         = "24h"
	WEEK        = "7d"
	MONTH       = "1m"
	SEASON      = "90d"
	HALFYEAR    = "180d"
	YEAR        = "1y"
	ADDRESS     = "address"
	CID         = "cid"
	HEIGHT      = "height"
	FNS         = "fns"
)

type AccountType string

func (a AccountType) Value() AccountType {
	switch a {
	case ACCOUNT:
		return a
	case MINER:
		return a
	case OWNER:
		return a
	case MULTISIG:
		return a
	case EVM:
		return a
	case ETHACCOUNT:
		return a
	case PLACEHOLDER:
		return a
	}
	return "empty"
}

type IntervalType string

func (i IntervalType) Value() IntervalType {
	switch i {
	case DAY:
		return i
	case WEEK:
		return i
	case MONTH:
		return i
	case SEASON:
		return i
	case HALFYEAR:
		return i
	case YEAR:
		return i
	default:
		return DAY
	}
}

type InputType string

func (i InputType) Value() InputType {
	switch i {
	case ADDRESS:
		return i
	case CID:
		return i
	case HEIGHT:
		return i
	}
	return "all_type"
}
