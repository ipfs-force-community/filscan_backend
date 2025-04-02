package londobell

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type Adapter interface {
	// Epoch http://192.168.1.57:3000/project/11/interface/api/11
	Epoch(ctx context.Context, epoch *chain.Epoch) (*EpochReply, error)
	// Actor http://192.168.1.57:3000/project/11/interface/api/17
	Actor(ctx context.Context, actorId chain.SmartAddress, epoch *chain.Epoch) (*ActorState, error)
	// Miner http://192.168.1.57:3000/project/11/interface/api/29
	Miner(ctx context.Context, miner chain.SmartAddress, epoch *chain.Epoch) (*MinerDetail, error)
	// ActorIDs http://192.168.1.57:3000/project/11/interface/api/91
	ActorIDs(ctx context.Context, epoch *chain.Epoch) (*ActorIDs, error)
	// MessagePool http://192.168.1.57:3000/project/11/interface/api/303
	//MessagePool(ctx context.Context, cid string, filters *types.Filters) (*MessagePool, error)
	// MinerList http://192.168.1.57:3000/project/11/interface/api/315
	MinerList(ctx context.Context, epoch *chain.Epoch) (*MinerList, error)
	// CurrentSectorInitialPledge http://192.168.1.57:3000/project/11/interface/api/319
	CurrentSectorInitialPledge(ctx context.Context, epoch *chain.Epoch) (*CurrentSectorInitialPledge, error)
	// ChangeActors http://192.168.1.57:3000/project/11/interface/api/443
	ChangeActors(ctx context.Context, epoch chain.Epoch) (reply map[string]*ChangeActorsReply, err error)
	// AllMethodsForMessagePool http://192.168.1.57:3000/project/11/interface/api/459
	//AllMethodsForMessagePool(ctx context.Context) ([]*MethodName, error)
	// LastEpoch http://192.168.1.57:3000/project/11/interface/api/471
	LastEpoch(ctx context.Context, epoch chain.Epoch) (reply *EpochReply, err error)
	// InitCodeForEVM http://192.168.1.57:3000/project/11/interface/api/515
	InitCodeForEVM(ctx context.Context, actorId chain.SmartAddress) (result *EVMByteCode, err error)
	// BalanceAtMarket 获取 miner Market 地址余额
	// http://192.168.1.57:3000/project/11/interface/api/1053
	BalanceAtMarket(ctx context.Context, miners []chain.SmartAddress, epoch chain.Epoch) (balances []*MarketBalance, err error)
	// 获取活跃扇区
	// http://192.168.1.57:3000/project/11/interface/api/1083
	ActiveSectors(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r *ActiveSectorsReply, err error)
}

type EpochReply struct {
	Cids            []cid.Cid       `json:"cids"`
	Parents         []cid.Cid       `json:"parents"`
	Epoch           int64           `json:"epoch"`
	BlockTime       time.Time       `json:"block_time"`
	BlockCount      int             `json:"block_count"`
	WinCount        int             `json:"win_count"`
	NetPower        string          `json:"net_power"`
	NetQualityPower string          `json:"net_quality_power"`
	NetRewards      string          `json:"net_rewards"`
	BaseFee         decimal.Decimal `json:"base_fee"`
	Source          string          `json:"source"`
}

type ActorState struct {
	ActorID       string          `json:"actor_id"`
	ActorAddr     string          `json:"actor_addr"`
	Epoch         int64           `json:"epoch"`
	BlockTime     time.Time       `json:"block_time"`
	ActorType     string          `json:"actor_type"`
	Balance       decimal.Decimal `json:"balance"`
	Code          cid.Cid         `json:"code"`
	Head          cid.Cid         `json:"head"`
	Nonce         int64           `json:"nonce"`
	State         interface{}     `json:"state"` // 保存 JSON 值，方便按 Actor 类型序列化
	DelegatedAddr string          `json:"delegated_addr,omitempty"`
}

type RewardActorState struct {
	CumsumBaseline          decimal.Decimal
	CumsumRealized          decimal.Decimal
	EffectiveNetworkTime    int64
	EffectiveBaselinePower  decimal.Decimal
	ThisEpochReward         decimal.Decimal
	ThisEpochRewardSmoothed *ThisEpochRewardSmoothed
	ThisEpochBaselinePower  decimal.Decimal
	Epoch                   int64
	TotalStoragePowerReward decimal.Decimal
	SimpleTotal             decimal.Decimal
	BaselineTotal           decimal.Decimal
}

type PowerActorState struct {
	TotalRawBytePower         decimal.Decimal
	TotalBytesCommitted       decimal.Decimal
	TotalQualityAdjPower      decimal.Decimal
	TotalQABytesCommitted     decimal.Decimal
	TotalPledgeCollateral     decimal.Decimal
	ThisEpochRawBytePower     decimal.Decimal
	ThisEpochQualityAdjPower  decimal.Decimal
	ThisEpochPledgeCollateral decimal.Decimal
	ThisEpochQAPowerSmoothed  *ThisEpochRewardSmoothed
	MinerCount                int64
	MinerAboveMinPowerCount   int64
	CronEventQueue            cid.Cid
	FirstCronEpoch            int64
	Claims                    cid.Cid
	ProofValidationBatch      interface{}
}

type ThisEpochRewardSmoothed struct {
	PositionEstimate decimal.Decimal
	VelocityEstimate decimal.Decimal
}

type MarketActorState struct {
	Proposals                     cid.Cid
	States                        cid.Cid
	PendingProposals              cid.Cid
	EscrowTable                   cid.Cid
	LockedTable                   cid.Cid
	NextID                        int64
	DealOpsByEpoch                cid.Cid
	LastCron                      int64
	TotalClientLockedCollateral   decimal.Decimal
	TotalProviderLockedCollateral decimal.Decimal
	TotalClientStorageFee         decimal.Decimal
}

type BurntActorState struct {
	Address string
}

type MutisigActorState struct {
	Signers               []string        `json:"Signers"`
	NumApprovalsThreshold int64           `json:"NumApprovalsThreshold"`
	NextTxnID             int64           `json:"NextTxnID"`
	InitialBalance        decimal.Decimal `json:"InitialBalance"`
	StartEpoch            int64           `json:"StartEpoch"`
	UnlockDuration        int64           `json:"UnlockDuration"`
	PendingTxns           cid.Cid         `json:"PendingTxns"`
}

type MinerDetail struct {
	Epoch                    int64           `json:"epoch"`
	Miner                    string          `json:"miner"`
	Owner                    string          `json:"owner"`
	Worker                   string          `json:"worker"`
	Controllers              []string        `json:"controllers"`
	SectorSize               int64           `json:"sector_size"`
	Power                    decimal.Decimal `json:"power"`
	QualityPower             decimal.Decimal `json:"quality_power"`
	Balance                  decimal.Decimal `json:"balance"`
	AvailableBalance         decimal.Decimal `json:"available_balance"`
	VestingFunds             decimal.Decimal `json:"vesting_funds"`              // 预存款
	LockedFunds              decimal.Decimal `json:"locked_funds"`               // 锁仓奖励(挖矿锁定)
	InitialPledgeRequirement decimal.Decimal `json:"initial_pledge_requirement"` // 扇区质押(初始抵押)
	State                    ChainMinerState `json:"state"`
	SectorCount              int64           `json:"sector_count"`
	FaultSectorCount         int64           `json:"fault_sector_count"`
	ActiveSectorCount        int64           `json:"active_sector_count"`
	LiveSectorCount          int64           `json:"live_sector_count"`
	RecoverSectorCount       int64           `json:"recover_sector_count"`
	TerminateSectorCount     int64           `json:"terminate_sector_count"`
	PreCommitSectorCount     int64           `json:"precommit_sector_count"`
	PeerID                   string          `json:"peerID"`
	Beneficiary              string          `json:"beneficiary"`
}

type ChainMinerState struct {
	Info              cid.Cid
	PreCommitDeposits decimal.Decimal
	LockedFunds       decimal.Decimal
	// 类型变了: 15 -> 16 : cid -> *VestingFunds
	// ...
	// VestingFunds               cid.Cid
	FeeDebt                    decimal.Decimal
	InitialPledge              decimal.Decimal
	PreCommittedSectors        cid.Cid
	PreCommittedSectorsCleanUp cid.Cid
	AllocatedSectors           cid.Cid
	Sectors                    cid.Cid
	ProvingPeriodStart         int64
	CurrentDeadline            int64
	Deadlines                  cid.Cid
	EarlyTerminations          []int64
	DeadlineCronActive         bool
}

type ActorIDs struct {
	ActorIds  []string  `json:"actor_ids"`
	Epoch     int       `json:"epoch"`
	BlockTime time.Time `json:"block_time"`
}

type PendingMessage struct {
	Cid        cid.Cid            `json:"Cid"`
	SignedCid  cid.Cid            `json:"SignedCid"`
	Epoch      chain.Epoch        `json:"Epoch"`
	From       chain.SmartAddress `json:"From"`
	To         chain.SmartAddress `json:"To"`
	Value      decimal.Decimal    `json:"Value"`
	GasLimit   int64              `json:"GasLimit"`
	GasPremium decimal.Decimal    `json:"GasPremium"`
	Method     string             `json:"Method"`
	Hash       string             `json:"Hash"`
	MsgTime    int64              `json:"MsgTime"`
}

type MessagePool struct {
	TotalCount     int64             `json:"TotalCount"`
	PendingMessage []*PendingMessage `json:"PendingMessages"`
}

type MinerList struct {
	ActorIds  []string  `json:"actor_ids"`
	Epoch     int64     `json:"epoch"`
	BlockTime time.Time `json:"block_time"`
}

type CurrentSectorInitialPledge struct {
	CirculatingRate            decimal.Decimal `json:"CirculatingRate"`
	FilVested                  decimal.Decimal `json:"FilVested"`
	FilMined                   decimal.Decimal `json:"FilMined"`
	FilBurnt                   decimal.Decimal `json:"FilBurnt"`
	FilLocked                  decimal.Decimal `json:"FilLocked"`
	FilCirculating             decimal.Decimal `json:"FilCirculating"`
	FilReserveDisbursed        decimal.Decimal `json:"FilReserveDisbursed"`
	CurrentSectorInitialPledge decimal.Decimal `json:"CurrentSectorInitialPledge"`
}

type ChangeActorsReply struct {
	Code    cid.Cid
	Head    cid.Cid
	Nonce   int64
	Balance decimal.Decimal
	Address string
}

type EVMByteCode struct {
	ByteCode string `json:"ByteCode"`
}

type MarketBalance struct {
	Actor         string          `json:"actor"`
	Epoch         int             `json:"epoch"`
	EscrowBalance decimal.Decimal `json:"escrow_balance"`
	LockedBalance decimal.Decimal `json:"locked_balance"`
}

type ActiveSectorsReply struct {
	SectorExpirations []*MinerSector  `json:"SectorExpirations"`
	VDCPower          decimal.Decimal `json:"VDCPower"`
	DCPower           decimal.Decimal `json:"DCPower"`
	CCPower           decimal.Decimal `json:"CCPower"`
}

type MinerSector struct {
	Expiration         int64           `json:"Expiration"`
	Activation         int64           `json:"Activation"`
	DealWeight         decimal.Decimal `json:"DealWeight"`
	VerifiedDealWeight decimal.Decimal `json:"VerifiedDealWeight"`
	InitialPledge      decimal.Decimal `json:"InitialPledge"`
}
