package filscan

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/shopspring/decimal"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type AccountAPI interface {
	AccountInfoByID(ctx context.Context, req AccountInfoByIDRequest) (resp AccountInfoByIDResponse, err error)
	AccountOwnerByID(ctx context.Context, req AccountOwnerByIDRequest) (resp AccountOwnerByIDResponse, err error)
	IndicatorsByAccountID(ctx context.Context, req IndicatorsByAccountIDRequest) (resp IndicatorsByAccountIDResponse, err error)
	BalanceTrendByAccountID(ctx context.Context, req BalanceTrendByAccountIDRequest) (resp BalanceTrendByAccountIDResponse, err error)
	PowerTrendByAccountID(ctx context.Context, req PowerTrendByAccountIDRequest) (resp PowerTrendByAccountIDResponse, err error)
	BlocksByAccountID(ctx context.Context, req BlocksByAccountIDRequest) (resp BlocksByAccountIDResponse, err error)
	MessagesByAccountID(ctx context.Context, req MessagesByAccountIDRequest) (resp MessagesByAccountIDResponse, err error)
	TracesByAccountID(ctx context.Context, req TracesByAccountIDRequest) (resp TracesByAccountIDResponse, err error)
	AllMethodByAccountID(ctx context.Context, req AllMethodByAccountIDRequest) (resp AllMethodByAccountIDResponse, err error)
	TransferMethodByAccountID(ctx context.Context, req AllMethodByAccountIDRequest) (resp AllTransferMethodByAccountIDResponse, err error)
	PendingMsgByAccount(ctx context.Context, req PendingMsgByAccountRequest) (resp MessagesPoolResponse, err error)
}

type AccountInfoByIDRequest struct {
	AccountID string         `json:"account_id"` // 账户ID(一般账户/多签账户/节点账户)
	Filters   *types.Filters `json:"filters"`
}

type AccountInfoByIDResponse struct {
	Epoch       int64       `json:"epoch"`
	AccountType string      `json:"account_type"`
	AccountInfo AccountInfo `json:"account_info"` // 账户信息
}

type AccountOwnerByIDRequest struct {
	OwnerID string `json:"owner_id"` // 所有者账户ID
}

type AccountOwnerByIDResponse struct {
	Epoch        int64         `json:"epoch"`
	AccountOwner *AccountOwner `json:"account_owner"` // 所有者账户信息
}

type IndicatorsByAccountIDRequest struct {
	AccountID string        `json:"account_id"` // AccountID
	Filters   types.Filters `json:"filters"`
}

type IndicatorsByAccountIDResponse struct {
	Epoch           int64            `json:"epoch"`
	MinerIndicators *MinerIndicators `json:"miner_indicators"` // 节点指标
}

type BalanceTrendByAccountIDRequest struct {
	AccountID string        `json:"account_id"` // AccountID
	Filters   types.Filters `json:"filters"`
}

type BalanceTrendByAccountIDResponse struct {
	Epoch                       int64           `json:"epoch"`
	BalanceTrendByAccountIDList []*BalanceTrend `json:"balance_trend_by_account_id_list"`
}

type PowerTrendByAccountIDRequest struct {
	AccountID string        `json:"account_id"` // AccountID
	Filters   types.Filters `json:"filters"`
}

type PowerTrendByAccountIDResponse struct {
	Epoch                     int64         `json:"epoch"`
	PowerTrendByAccountIDList []*PowerTrend `json:"power_trend_by_account_id_list"`
}

type PendingMsgByAccountRequest struct {
	AccountID   string `json:"account_id"`
	AccountAddr string `json:"account_address"`
}

type BlocksByAccountIDRequest struct {
	AccountID string        `json:"account_id"` // AccountID
	Filters   types.Filters `json:"filters"`
}

type BlocksByAccountIDResponse struct {
	BlocksByAccountIDList []*BlockBasic `json:"blocks_by_account_id_list"`
	TotalCount            int64         `json:"total_count"`
}

type MessagesByAccountIDRequest struct {
	AccountID string        `json:"account_id"` // AccountID
	Filters   types.Filters `json:"filters"`
}

type MessagesByAccountIDResponse struct {
	Epoch                   int64           `json:"epoch"`
	MessagesByAccountIDList []*MessageBasic `json:"messages_by_account_id_list"`
	TotalCount              int64           `json:"total_count"`
}

type TracesByAccountIDRequest struct {
	AccountID string        `json:"account_id"` // AccountID
	Filters   types.Filters `json:"filters"`
}

type TracesByAccountIDResponse struct {
	Epoch                 int64           `json:"epoch"`
	TracesByAccountIDList []*MessageBasic `json:"traces_by_account_id_list"`
	TotalCount            int64           `json:"total_count"`
}

type AllMethodByAccountIDRequest struct {
	AccountID string `json:"account_id"` // AccountID
}

type AllTransferMethodByAccountIDResponse struct {
	MethodNameList []string `json:"method_name_list"`
}

type AllMethodByAccountIDResponse struct {
	Epoch          int64            `json:"epoch"`
	MethodNameList map[string]int64 `json:"method_name_list"`
}

// -----------------------账户页基础数据结构-----------------------

type AccountInfo struct {
	AccountBasic    *AccountBasic    `json:"account_basic,omitempty"`    // 账户基本信息(一般账户/支付通道)
	AccountMiner    *AccountMiner    `json:"account_miner,omitempty"`    // 节点账户基本信息
	AccountMultisig *AccountMultisig `json:"account_multisig,omitempty"` // 多签账户基本信息
}

type AccountBasic struct {
	AccountID          string          `json:"account_id"`              // 账户ID(在多签账户中为账户地址)
	AccountAddress     string          `json:"account_address"`         // 账户地址(在多签账户中为Robust Address)
	AccountType        string          `json:"account_type"`            // 账户类型
	AccountBalance     decimal.Decimal `json:"account_balance"`         // 账户余额
	MessageCount       int64           `json:"message_count,omitempty"` // 消息总数
	Nonce              int64           `json:"nonce"`                   // Nonce数
	CodeCid            string          `json:"code_cid"`                // 代码Cid
	CreateTime         *int64          `json:"create_time"`             // 创建时间
	LatestTransferTime *int64          `json:"latest_transfer_time"`    // 最新交易时间
	OwnedMiners        []string        `json:"owned_miners,omitempty"`  // 名下节点ID
	ActiveMiners       []string        `json:"active_miners,omitempty"`
	EthAddress         string          `json:"eth_address,omitempty"`    // Eth地址
	StableAddress      string          `json:"stable_address,omitempty"` // Eth账号的稳定地址
	EvmContract        *EvmContract    `json:"evm_contract,omitempty"`   // evm账号合约信息
}

type AccountIndicator struct {
	AccountID              string          `json:"account_id"`               // 账户ID
	Balance                decimal.Decimal `json:"balance"`                  // 账户总余额
	AvailableBalance       decimal.Decimal `json:"available_balance"`        // 可用余额
	InitPledge             decimal.Decimal `json:"init_pledge"`              // 扇区质押(初始抵押)
	PreDeposits            decimal.Decimal `json:"pre_deposits"`             // 预存款
	LockedBalance          decimal.Decimal `json:"locked_balance"`           // 锁仓奖励(挖矿锁定)
	QualityAdjustPower     decimal.Decimal `json:"quality_adjust_power"`     // 有效算力
	QualityPowerRank       int64           `json:"quality_power_rank"`       // 有效算力排名
	QualityPowerPercentage decimal.Decimal `json:"quality_power_percentage"` // 有效算力占比
	RawPower               decimal.Decimal `json:"raw_power"`                // 原值算力
	TotalBlockCount        int64           `json:"total_block_count"`        // 总出块数
	TotalWinCount          int64           `json:"total_win_count"`          // 总赢票数
	TotalReward            decimal.Decimal `json:"total_reward"`             // 总出块奖励
	SectorSize             int64           `json:"sector_size"`              // 扇区大小
	SectorCount            int64           `json:"sector_count"`             // 扇区总数
	LiveSectorCount        int64           `json:"live_sector_count"`        // 活着扇区(全部扇区)
	FaultSectorCount       int64           `json:"fault_sector_count"`       // 错误扇区
	RecoverSectorCount     int64           `json:"recover_sector_count"`     // 恢复扇区
	ActiveSectorCount      int64           `json:"active_sector_count"`      // 活跃扇区
	TerminateSectorCount   int64           `json:"terminate_sector_count"`   // 终止扇区
}

type AccountMiner struct {
	AccountBasic       *AccountBasic     `json:"account_basic"`       // 账户基本信息
	AccountIndicator   *AccountIndicator `json:"account_indicator"`   // 存储池或节点概览
	PeerID             string            `json:"peer_id"`             // 节点标识
	OwnerAddress       string            `json:"owner_address"`       // Owner地址
	WorkerAddress      string            `json:"worker_address"`      // Worker地址
	ControllersAddress []string          `json:"controllers_address"` // Controllers地址列表
	BeneficiaryAddress string            `json:"beneficiary_address"` // Beneficiary地址列表
	IpAddress          string            `json:"ip_address"`          // 地区
}

type AccountMultisig struct {
	AccountBasic            *AccountBasic   `json:"account_basic"`             // 账户基本信息
	AvailableBalance        decimal.Decimal `json:"available_balance"`         // 可用余额
	InitialBalance          decimal.Decimal `json:"initial_balance"`           // 账户初始余额
	UnlockStartTime         int64           `json:"unlock_start_time"`         // 解锁起始时间
	UnlockEndTime           int64           `json:"unlock_end_time"`           // 解锁终止时间
	LockedBalance           decimal.Decimal `json:"locked_balance"`            // 锁仓奖励(挖矿锁定)
	LockedBalancePercentage decimal.Decimal `json:"locked_balance_percentage"` // 锁仓奖励占比
	Signers                 []string        `json:"signers"`                   // 签名者地址
	ApprovalsThreshold      int64           `json:"approvals_threshold"`       // Approval阈值
}

type AccountOwner struct {
	AccountID        string            `json:"account_id"`             // OwnerID
	AccountAddress   string            `json:"account_address"`        // Owner地址
	OwnedMiners      []string          `json:"owned_miners,omitempty"` // 名下节点ID
	ActiveMiners     []string          `json:"active_miners,omitempty"`
	AccountIndicator *AccountIndicator `json:"account_indicator"` // 存储池或节点概览
}

type MinerIndicators struct {
	PowerIncrease       decimal.Decimal `json:"power_increase"`        // 算力增量
	PowerRatio          decimal.Decimal `json:"power_ratio"`           // 算力增速
	SectorIncrease      decimal.Decimal `json:"sector_increase"`       // 扇区增量
	SectorRatio         decimal.Decimal `json:"sector_ratio"`          // 扇区增速
	SectorDeposits      decimal.Decimal `json:"sector_deposits"`       // 扇区抵押
	GasFee              decimal.Decimal `json:"gas_fee"`               // gas消耗
	BlockCountIncrease  int64           `json:"block_count_increase"`  // 出块增量
	BlockRewardIncrease decimal.Decimal `json:"block_reward_increase"` // 出块奖励
	WinCount            int64           `json:"win_count"`             // 赢票数量
	RewardsPerTB        decimal.Decimal `json:"rewards_per_tb"`        // 效率(出块奖励/有效算力:FIL/TiB)
	GasFeePerTB         decimal.Decimal `json:"gas_fee_per_tb"`        // 单T消耗(gas消耗/扇区增量:FIL/TiB)
	Lucky               decimal.Decimal `json:"lucky"`                 // 幸运值
	WindowPoStGas       decimal.Decimal `json:"windowpost_gas"`        // 维持算力消耗
}

type Consume struct {
	From            string          `json:"from"` // 发送方地址
	To              string          `json:"to"`   // 接收方地址
	FromTag         string          `json:"from_tag"`
	ToTag           string          `json:"to_tag"`
	Value           decimal.Decimal `json:"value"`                       // 发送金额
	ConsumeType     string          `json:"consume_type"`                // 1.矿工手续费 2.销毁手续费 3.转账 4.销毁
	ConsumeEnumType int             `json:"consume_enum_type,omitempty"` // 类型枚举 1:销毁手续费 2:矿工手续费 3:转账 4:惩罚 5:举报 6:聚合费用
}

func (c Consume) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		From            string          `json:"from"` // 发送方地址
		To              string          `json:"to"`   // 接收方地址
		FromTag         string          `json:"from_tag"`
		ToTag           string          `json:"to_tag"`
		Value           decimal.Decimal `json:"value"`                       // 发送金额
		ConsumeType     string          `json:"consume_type"`                // 1.矿工手续费 2.销毁手续费 3.转账 4.销毁
		ConsumeEnumType int             `json:"consume_enum_type,omitempty"` // 类型枚举 1:销毁手续费 2:矿工手续费 3:转账 4:惩罚 5:举报 6:聚合费用
	}{
		From:            c.From,
		To:              c.To,
		FromTag:         convertor.GlobalTagMap[c.From],
		ToTag:           convertor.GlobalTagMap[c.To],
		Value:           c.Value,
		ConsumeType:     c.ConsumeType,
		ConsumeEnumType: c.ConsumeEnumType,
	})
}

type BalanceTrend struct {
	Height            *big.Int         `json:"height"`                       // 高度
	BlockTime         int64            `json:"block_time"`                   // 区块时间
	Balance           decimal.Decimal  `json:"balance"`                      // 当前余额
	AvailableBalance  *decimal.Decimal `json:"available_balance,omitempty"`  // 可用余额
	InitialPledge     *decimal.Decimal `json:"initial_pledge,omitempty"`     // 初始抵押(扇区抵押)
	LockedFunds       *decimal.Decimal `json:"locked_funds,omitempty"`       // 锁仓奖励
	PreCommitDeposits *decimal.Decimal `json:"precommit_deposits,omitempty"` // 预存款
	Epoch             int64            `json:"epoch,omitempty"`
}

type PowerTrend struct {
	BlockTime     int64           `json:"block_time"`     // 区块时间
	Power         decimal.Decimal `json:"power"`          // 有效算力
	PowerIncrease decimal.Decimal `json:"power_increase"` // 有效算力增长
	Epoch         int64           `json:"epoch"`
}
