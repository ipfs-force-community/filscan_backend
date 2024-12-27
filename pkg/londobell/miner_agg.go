package londobell

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type MinerAgg interface {
	// PeriodBlockRewards 用于返回高度范围内指定矿工的总blockreward和总出块数，包括BlockRewards、 BlockCounts。
	// http://192.168.1.57:3000/project/19/interface/api/693
	PeriodBlockRewards(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *PeriodBlockRewardsResp, err error)
	// PeriodWinCounts 用于返回高度范围内指定矿工的总wincout和总gasreward，包括WinCounts、 GasRewards。
	// http://192.168.1.57:3000/project/19/interface/api/696
	PeriodWinCounts(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *PeriodWinCountsResp, err error)
	// PeriodGasCost 用于返回高度范围内指定矿工的各个消息的gas消耗情况，包括_id、GasCosts、Values。
	// http://192.168.1.57:3000/project/19/interface/api/699
	PeriodGasCost(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r []*PeriodGasCostResp, err error)
	// PeriodGasCostForPublishDeals 用于返回高度范围内指定矿工的发单gas消耗情况，包括_id、GasCosts、Values。
	// http://192.168.1.57:3000/project/19/interface/api/705
	PeriodGasCostForPublishDeals(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r []*PeriodGasCostResp, err error)
	// PeriodPunishments 用于返回高度范围内指定矿工的总惩罚，包括Punishments。
	// http://192.168.1.57:3000/project/19/interface/api/708
	PeriodPunishments(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *PeriodPunishmentsResp, err error)
	// PeriodSectorDiff 用于返回高度范围内指定矿工的扇区变化情况，包括AllSectorsDiff、LiveSectorsDiff、LiveQAPowerDiff、LiveRawPowerDiff、FaultSectorsDiff、FaultQAPowerDiff、FaultRawPowerDiff。
	// http://192.168.1.57:3000/project/19/interface/api/711
	PeriodSectorDiff(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *PeriodSectorDiffResp, err error)
	// PeriodPledgeDiff 用于返回高度范围内指定矿工的质押变化情况，包括InitialPledgeDiff、PreCommitDepositsDiff、LockedFundsDiff。
	// http://192.168.1.57:3000/project/19/interface/api/714
	PeriodPledgeDiff(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *PeriodPledgeDiffResp, err error)
	// PeriodExpirations 用于返回高度范围内指定矿工的扇区过期情况，包括Miner、SectorNumber、Activation、Expiration、InitialPledge、DealWeight、VerifiedDealWeight、DealIDs、SimpleQaPower。
	// http://192.168.1.57:3000/project/19/interface/api/717
	PeriodExpirations(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r []*Expiration, err error)
	// QaPowerHistory 用于返回高度指定矿工的有效算力情况，包括VDCPower、DCPower、CCPower。
	// http://192.168.1.57:3000/project/19/interface/api/720
	QaPowerHistory(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r *QaPowerHistoryResp, err error)
	// SectorHealthHistory 用于返回请求高度的扇区情况，包括AllSectors、LiveSectors、LiveQAPower、LiveRawPower、FaultSectors、FaultQAPower、FaultRawPower 
	// http://192.168.1.57:3000/project/19/interface/api/1005
	SectorHealthHistory(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r *SectorHealthHistoryResp, err error)
	// PeriodBill 获取地址的总支出和手续费明细
	PeriodBill(ctx context.Context, addr chain.SmartAddress, epochs chain.LCRORange) (r *PeriodBillResp, err error)
}

type PeriodBlockRewardsResp struct {
	BlockRewards decimal.Decimal `json:"BlockRewards"`
	BlockCounts  int64           `json:"BlockCounts"`
}

type PeriodWinCountsResp struct {
	WinCounts  int64           `json:"WinCounts"`
	GasRewards decimal.Decimal `json:"GasRewards"`
}

type PeriodGasCostResp struct {
	Method   string          `json:"_id"`
	GasCosts decimal.Decimal `json:"GasCosts"`
	Values   decimal.Decimal `json:"Values"`
}

type PeriodPunishmentsResp struct {
	Punishments decimal.Decimal `json:"Punishments"`
}

type PeriodSectorDiffResp struct {
	AllSectorsDiff    int64           `json:"AllSectorsDiff"`
	LiveSectorsDiff   int64           `json:"LiveSectorsDiff"`
	LiveQAPowerDiff   decimal.Decimal `json:"LiveQAPowerDiff"`
	LiveRawPowerDiff  decimal.Decimal `json:"LiveRawPowerDiff"`
	FaultSectorsDiff  int64           `json:"FaultSectorsDiff"`
	FaultQAPowerDiff  decimal.Decimal `json:"FaultQAPowerDiff"`
	FaultRawPowerDiff decimal.Decimal `json:"FaultRawPowerDiff"`
}

type PeriodPledgeDiffResp struct {
	InitialPledgeDiff     decimal.Decimal `json:"InitialPledgeDiff"`
	PreCommitDepositsDiff decimal.Decimal `json:"PreCommitDepositsDiff"`
	LockedFundsDiff       decimal.Decimal `json:"LockedFundsDiff"`
}

type Expiration struct {
	Miner              chain.SmartAddress `json:"Miner"`
	SectorNumber       int64              `json:"SectorNumber"`
	Activation         chain.Epoch        `json:"Activation"`
	Expiration         chain.Epoch        `json:"Expiration"`
	InitialPledge      decimal.Decimal    `json:"InitialPledge"`
	DealWeight         decimal.Decimal    `json:"DealWeight"`
	VerifiedDealWeight decimal.Decimal    `json:"VerifiedDealWeight"`
	DealIDs            []int64            `json:"DealIDs"`
	SimpleQaPower      bool               `json:"SimpleQaPower"`
}

type QaPowerHistoryResp struct {
	VDCPower decimal.Decimal `json:"VDCPower"`
	DCPower  decimal.Decimal `json:"DCPower"`
	CCPower  decimal.Decimal `json:"CCPower"`
}

type SectorHealthHistoryResp struct {
	AllSectors    int64  `json:"AllSectors"`
	LiveSectors   int64  `json:"LiveSectors"`
	LiveQAPower   string `json:"LiveQAPower"`
	LiveRawPower  string `json:"LiveRawPower"`
	FaultSectors  int64  `json:"FaultSectors"`
	FaultQAPower  string `json:"FaultQAPower"`
	FaultRawPower string `json:"FaultRawPower"`
}

type PeriodBillResp struct {
	Income  decimal.Decimal `json:"Income"`
	Pay     decimal.Decimal `json:"Pay"`
	GasCost decimal.Decimal `json:"GasCost"`
}
