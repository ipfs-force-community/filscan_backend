package pro

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"time"
)

type MinerAPI interface {
	MinerInfoDetail(ctx context.Context, req MinerInfoDetailRequest) (response MinerInfoDetailResponse, err error)
	PowerDetail(ctx context.Context, req PowerDetailRequest) (response PowerDetailResponse, err error)
	GasCostDetail(ctx context.Context, req GasCostDetailRequest) (response GasCostDetailResponse, err error)
	SectorDetail(ctx context.Context, req SectorDetailRequest) (response SectorDetailResponse, err error)
	RewardDetail(ctx context.Context, req RewardDetailRequest) (response RewardDetailResponse, err error)
	LuckyRateDetail(ctx context.Context, req LuckyRateDetailRequest) (response LuckyRateDetailResponse, err error)
	BalanceDetail(ctx context.Context, req BalanceDetailRequest) (response BalanceDetailResponse, err error)
}

type MinerInfoDetailRequest struct {
	GroupID int64 `json:"group_id"`
}

type MinerInfoDetailResponse struct {
	Epoch               chain.Epoch        `json:"epoch"`
	EpochTime           time.Time          `json:"epoch_time"`
	SumQualityAdjPower  decimal.Decimal    `json:"sum_quality_adj_power"`
	SumPowerChange24h   decimal.Decimal    `json:"sum_power_change_24h"`
	SumReward           decimal.Decimal    `json:"sum_reward"`
	SumRewardChange24h  decimal.Decimal    `json:"sum_reward_change_24h"`
	SumOutlay           decimal.Decimal    `json:"sum_outlay"`
	SumGas              decimal.Decimal    `json:"sum_gas"`
	SumPledge           decimal.Decimal    `json:"sum_pledge"`
	SumPledgeChange24h  decimal.Decimal    `json:"sum_pledge_change_24h"`
	SumBalance          decimal.Decimal    `json:"sum_balance"`
	SumBalanceChange24h decimal.Decimal    `json:"sum_balance_change_24h"`
	MinerInfoDetailList []*MinerInfoDetail `json:"miner_info_detail_list"`
}

type PowerDetailRequest struct {
	GroupID   int64               `json:"group_id"`
	MinerID   *chain.SmartAddress `json:"miner_id"`
	StartDate *string             `json:"start_date"`
	EndDate   *string             `json:"end_date"`
}

type PowerDetailResponse struct {
	Epoch           chain.Epoch    `json:"epoch"`
	EpochTime       time.Time      `json:"epoch_time"`
	PowerDetailList []*PowerDetail `json:"power_detail_list"`
}

type GasCostDetailRequest struct {
	GroupID   int64               `json:"group_id"`
	MinerID   *chain.SmartAddress `json:"miner_id"`
	StartDate *string             `json:"start_date"`
	EndDate   *string             `json:"end_date"`
}

type GasCostDetailResponse struct {
	Epoch             chain.Epoch      `json:"epoch"`
	EpochTime         time.Time        `json:"epoch_time"`
	GasCostDetailList []*GasCostDetail `json:"gas_cost_detail_list"`
}

type SectorDetailRequest struct {
	GroupID int64               `json:"group_id"`
	MinerID *chain.SmartAddress `json:"miner_id"`
}

type SectorDetailResponse struct {
	Epoch             chain.Epoch                 `json:"epoch"`
	EpochTime         time.Time                   `json:"epoch_time"`
	SectorDetailMonth []*SectorDetailMonth        `json:"sector_detail_month"`
	SectorDetailDay   []*SectorDetail             `json:"sector_detail_day"`
	Summary           SectorDetailResponseSummary `json:"summary"`
}

type SectorDetailResponseSummary struct {
	TotalPower decimal.Decimal `json:"total_power"`
	TotalDc    decimal.Decimal `json:"total_dc"`
	TotalCC    decimal.Decimal `json:"total_cc"`
}

type RewardDetailRequest struct {
	GroupID   int64               `json:"group_id"`
	MinerID   *chain.SmartAddress `json:"miner_id"`
	StartDate *string             `json:"start_date"`
	EndDate   *string             `json:"end_date"`
}

type RewardDetailResponse struct {
	Epoch            chain.Epoch     `json:"epoch"`
	EpochTime        time.Time       `json:"epoch_time"`
	RewardDetailList []*RewardDetail `json:"reward_detail_list"`
}

type LuckyRateDetailRequest struct {
	GroupID int64               `json:"group_id"`
	MinerID *chain.SmartAddress `json:"miner_id"`
}

type LuckyRateDetailResponse struct {
	Epoch         chain.Epoch        `json:"epoch"`
	EpochTime     time.Time          `json:"epoch_time"`
	LuckyRateList []*LuckyRateDetail `json:"lucky_rate_list"`
}

type BalanceDetailRequest struct {
	GroupID int64               `json:"group_id"`
	MinerID *chain.SmartAddress `json:"miner_id"`
}

type BalanceDetailResponse struct {
	Epoch             chain.Epoch      `json:"epoch"`
	EpochTime         time.Time        `json:"epoch_time"`
	BalanceDetailList []*BalanceDetail `json:"address_balance_list"`
}

type MinerInfoDetail struct {
	Tag                  string             `json:"tag"`
	MinerId              chain.SmartAddress `json:"miner_id"`
	GroupName            string             `json:"group_name"`
	IsDefault            bool               `json:"is_default"`
	TotalQualityAdjPower decimal.Decimal    `json:"total_quality_adj_power"`
	TotalRawBytePower    decimal.Decimal    `json:"total_raw_byte_power"`
	TotalReward          decimal.Decimal    `json:"total_reward"`
	RewardChange24h      decimal.Decimal    `json:"reward_change_24h"`
	TotalOutlay          decimal.Decimal    `json:"total_outlay"`
	TotalGas             decimal.Decimal    `json:"total_gas"`
	TotalPledge          decimal.Decimal    `json:"total_pledge_amount"`
	PledgeChange24h      decimal.Decimal    `json:"pledge_change_24h"`
	TotalBalance         decimal.Decimal    `json:"total_balance"`
	BalanceChange24h     decimal.Decimal    `json:"balance_change_24h"`
}

type PowerDetail struct {
	Date              time.Time          `json:"date"`
	Tag               string             `json:"tag"`
	MinerId           chain.SmartAddress `json:"miner_id"`
	GroupName         string             `json:"group_name"`
	IsDefault         bool               `json:"is_default"`
	QualityPower      decimal.Decimal    `json:"quality_power"`
	RawPower          decimal.Decimal    `json:"raw_power"`
	DCPower           decimal.Decimal    `json:"dc_power"`
	CCPower           decimal.Decimal    `json:"cc_power"`
	SectorSize        decimal.Decimal    `json:"sector_size"`
	SectorPowerChange decimal.Decimal    `json:"sector_power_change"`
	SectorCountChange int64              `json:"sector_count_change"`
	PledgeChanged     decimal.Decimal    `json:"pledge_changed"`
	PledgeChangedPerT decimal.Decimal    `json:"pledge_changed_per_t"`
	Penalty           decimal.Decimal    `json:"penalty"`
	FaultSectors      int64              `json:"fault_sectors"`
}

type GasCostDetail struct {
	Date              string             `json:"date"`
	Tag               string             `json:"tag"`
	MinerId           chain.SmartAddress `json:"miner_id"`
	GroupName         string             `json:"group_name"`
	IsDefault         bool               `json:"is_default"`
	SectorPowerChange decimal.Decimal    `json:"sector_power_change"`
	SectorCountChange int64              `json:"sector_count_change"`
	TotalGasCost      decimal.Decimal    `json:"total_gas_cost"`
	SealGasCost       decimal.Decimal    `json:"seal_gas_cost"`
	SealGasPerT       decimal.Decimal    `json:"seal_gas_per_t"`
	DealGasCost       decimal.Decimal    `json:"deal_gas_cost"`
	WdPostGasCost     decimal.Decimal    `json:"wd_post_gas_cost"`
	WdPostGasPerT     decimal.Decimal    `json:"wd_post_gas_per_t"`
}

type RewardDetail struct {
	Date        string             `json:"date"`
	Tag         string             `json:"tag"`
	MinerId     chain.SmartAddress `json:"miner_id"`
	GroupName   string             `json:"group_name"`
	IsDefault   bool               `json:"is_default"`
	BlockCount  int64              `json:"block_count"`
	WinCount    int64              `json:"win_count"`
	BlockReward decimal.Decimal    `json:"block_reward"`
	TotalReward decimal.Decimal    `json:"total_reward"`
}

type SectorDetailMonth struct {
	ExpMonth            string          `json:"exp_month"`
	TotalMinerCount     int64           `json:"total_miner_count"`
	TotalExpPower       decimal.Decimal `json:"total_exp_power"`
	TotalExpSectorCount int64           `json:"total_exp_sector_count"`
	TotalExpDC          decimal.Decimal `json:"total_exp_dc"`
	TotalExpPledge      decimal.Decimal `json:"total_exp_pledge"`
	SectorDetailList    []*SectorDetail `json:"sector_detail_list"`
}

type SectorDetail struct {
	ExpDate        string             `json:"exp_date,omitempty"`
	Tag            string             `json:"tag"`
	MinerId        chain.SmartAddress `json:"miner_id"`
	GroupName      string             `json:"group_name"`
	IsDefault      bool               `json:"is_default"`
	ExpPower       decimal.Decimal    `json:"exp_power"`
	ExpSectorCount int64              `json:"exp_sector_count"`
	ExpDC          decimal.Decimal    `json:"exp_dc"`
	ExpPledge      decimal.Decimal    `json:"exp_pledge"`
}

type BalanceDetail struct {
	Tag                       string             `json:"tag"`
	MinerID                   chain.SmartAddress `json:"miner_id"`
	GroupName                 string             `json:"group_name"`
	IsDefault                 bool               `json:"is_default"`
	MinerBalance              decimal.Decimal    `json:"miner_balance"`
	MinerBalanceChanged       decimal.Decimal    `json:"miner_balance_changed"`
	OwnerBalance              decimal.Decimal    `json:"owner_balance"`
	OwnerBalanceChanged       decimal.Decimal    `json:"Owner_balance_changed"`
	WorkerBalance             decimal.Decimal    `json:"worker_balance"`
	WorkerBalanceChanged      decimal.Decimal    `json:"Worker_balance_changed"`
	Controller0Balance        decimal.Decimal    `json:"controller_0_balance"`
	Controller0BalanceChanged decimal.Decimal    `json:"Controller_0_balance_changed"`
	Controller1Balance        decimal.Decimal    `json:"controller_1_balance"`
	Controller1BalanceChanged decimal.Decimal    `json:"controller_1_balance_changed"`
	Controller2Balance        decimal.Decimal    `json:"controller_2_balance"`
	Controller2BalanceChanged decimal.Decimal    `json:"controller_2_balance_changed"`
	BeneficiaryBalance        decimal.Decimal    `json:"beneficiary_balance"`
	BeneficiaryBalanceChanged decimal.Decimal    `json:"beneficiary_balance_changed"`
	MarketBalance             decimal.Decimal    `json:"market_balance"`
	MarketBalanceChanged      decimal.Decimal    `json:"market_balance_changed"`
}

type LuckyRateDetail struct {
	Tag          string             `json:"tag"`
	MinerID      chain.SmartAddress `json:"miner_id"`
	GroupName    string             `json:"group_name"`
	IsDefault    bool               `json:"is_default"`
	LuckyRate24h decimal.Decimal    `json:"lucky_rate_24h"`
	LuckyRate7d  decimal.Decimal    `json:"lucky_rate_7d"`
	LuckyRate30d decimal.Decimal    `json:"lucky_rate_30d"`
	//LuckyRate365d decimal.Decimal    `json:"lucky_rate_356d"`
}
