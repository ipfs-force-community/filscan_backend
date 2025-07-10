package merger

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type Merger interface {
	// AGG Trace 最新高度
	TraceHeight(ctx context.Context) (epoch chain.Epoch, err error)
	
	// AGG State 时间
	StateHeight(ctx context.Context) (epoch chain.Epoch, err error)
	
	// Adapter 最新高度
	AdapterHeight(ctx context.Context) (epoch chain.Epoch, err error)
	
	MinersInfos(ctx context.Context, miners []chain.SmartAddress, date chain.Date) (epoch chain.Epoch, summary MinersSummary, infos map[chain.SmartAddress]*MinerInfo, err error)
	
	// 获取 miners 在某一个点的算力情况
	MinersPowerStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*DayPowerStat, err error)
	
	// 获取 miners 列表金融相关的统计
	MinersFundStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*DayFundStat, err error)
	
	// 获取 miners 爆块奖励统计 
	MinersRewardStats(ctx context.Context, miners []chain.SmartAddress, dates chain.DateLCRCRange) (epoch chain.Epoch, stats []*DayRewardStat, err error)
	
	// 获取 miners 的扇区统计
	MinersSectorStats(ctx context.Context, miners []chain.SmartAddress) (epoch chain.Epoch, stats *SectorStat, err error)
	
	// 获取 miners 的幸运值统计
	MinersLuckStats(ctx context.Context, miners []chain.SmartAddress) (epoch chain.Epoch, stats map[chain.SmartAddress]*LuckStats, err error)
	
	// 获取 miners 的余额统计
	MinersBalanceStats(ctx context.Context, miners []chain.SmartAddress, date chain.Date) (epoch chain.Epoch, stats map[chain.SmartAddress]*BalanceStat, err error)
}

type MinerInfo struct {
	QualityAdjPower     chain.Byte
	QualityAdjPowerZero chain.Byte
	RawBytePower        chain.Byte
	Reward              chain.AttoFil
	RewardZero          chain.AttoFil
	Outlay              chain.AttoFil
	Gas                 chain.AttoFil
	PledgeAmount        chain.AttoFil
	PledgeZero          chain.AttoFil
	Balance             chain.AttoFil
	BalanceZero         chain.AttoFil
}

type MinersSummary struct {
	TotalQualityAdjPower     chain.Byte
	TotalQualityAdjPowerZero chain.Byte
	TotalReward              chain.AttoFil
	TotalRewardZero          chain.AttoFil
	TotalOutcome             chain.AttoFil
	TotalGas                 chain.AttoFil
	TotalPledge              chain.AttoFil
	TotalPledgeZero          chain.AttoFil
	TotalBalance             chain.AttoFil
	TotalBalanceZero         chain.AttoFil
}

type DayPowerStat struct {
	Day   chain.Date
	Stats map[chain.SmartAddress]*PowerStat
}

type PowerStat struct {
	Miner                 chain.SmartAddress
	QualityAdjPower       chain.Byte
	RawBytePower          chain.Byte
	VdcPower              chain.Byte
	CcPower               chain.Byte
	SectorSize            chain.Byte
	TotalSectors          int64
	TotalSectorsZero      int64
	TotalSectorsPowerZero chain.Byte
	PledgeAmountZero      chain.AttoFil
	PledgeAmountZeroPert  chain.AttoFil
	PenaltyZero           chain.AttoFil
	FaultSectors          int64
}

type DayFundStat struct {
	Day   chain.Date
	Stats map[chain.SmartAddress]*FundStat
}

type FundStat struct {
	Miner                 chain.SmartAddress
	TotalSectorsZero      int64
	TotalSectorsPowerZero chain.Byte
	TotalGas              chain.AttoFil
	SealGas               chain.AttoFil
	SealGasPerT           chain.AttoFil
	PublishDealGas        chain.AttoFil
	WdPostGas             chain.AttoFil
	WdPostGasPerT         chain.AttoFil
}

type DayRewardStat struct {
	Day   chain.Date
	Stats map[chain.SmartAddress]*RewardStat
}

type RewardStat struct {
	Miner        chain.SmartAddress
	Blocks       int64
	WinCounts    int64
	Rewards      chain.AttoFil
	TotalRewards chain.AttoFil
}

type SectorStat struct {
	Months []*MonthSectorStat
	Days   []*DaySectorStat
}

type MinerSectorStat struct {
	Miner   chain.SmartAddress
	Sectors int64
	Power   chain.Byte
	VDC     chain.Byte
	DC      chain.Byte
	CC      chain.Byte
	Pledge  chain.AttoFil
}

type MonthSectorStat struct {
	Month string
	MinerSectorStat
	Miners []*MinerSectorStat
}

type DaySectorStat struct {
	Day string
	MinerSectorStat
	Miners []*MinerSectorStat
}

type LuckStats struct {
	Luck24h  decimal.Decimal
	Luck7d   decimal.Decimal
	Luck30d  decimal.Decimal
	Luck365d *decimal.Decimal // 截止：2023年08月15日14:33:58 无数据
}

type BalanceStat struct {
	Addr            chain.SmartAddress
	Miner           chain.AttoFil
	MinerZero       chain.AttoFil
	Owner           chain.AttoFil
	OwnerZero       chain.AttoFil
	Worker          chain.AttoFil
	WorkerZero      chain.AttoFil
	C0              chain.AttoFil
	C0Zero          chain.AttoFil
	C1              chain.AttoFil
	C1Zero          chain.AttoFil
	C2              chain.AttoFil
	C2Zero          chain.AttoFil
	Beneficiary     chain.AttoFil
	BeneficiaryZero chain.AttoFil
	Market          chain.AttoFil
	MarketZero      chain.AttoFil
}
