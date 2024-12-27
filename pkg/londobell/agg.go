package londobell

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Agg interface {
	// Address http://192.168.1.57:3000/project/13/interface/api/39
	Address(ctx context.Context, addr chain.SmartAddress) (*Address, error)
	// AggPreNetFee http://192.168.1.57:3000/project/13/interface/api/43
	AggPreNetFee(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*AggPreNetFee, error)
	// AggProNetFee http://192.168.1.57:3000/project/13/interface/api/47
	AggProNetFee(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*AggProNetFee, error)
	// BlockMessages http://192.168.1.57:3000/project/13/interface/api/51
	BlockMessages(ctx context.Context, filter types.Filters) (result *BlockMessagesList, err error)
	// MinerBlockReward http://192.168.1.57:3000/project/13/interface/api/55
	MinerBlockReward(ctx context.Context, addr chain.SmartAddress, filter types.Filters) ([]*MinerBlockReward, error)
	// FinalHeight http://192.168.1.57:3000/project/13/interface/api/63
	FinalHeight(ctx context.Context) (epoch *chain.Epoch, err error)
	// StateFinalHeight http://106.15.125.51:2345/aggregators/state_final_height
	StateFinalHeight(ctx context.Context) (epoch *chain.Epoch, err error)
	// MinersInfo http://192.168.1.57:3000/project/13/interface/api/67
	MinersInfo(ctx context.Context, start, end chain.Epoch) (r []*MinerInfo, err error)
	// WinCount http://192.168.1.57:3000/project/13/interface/api/79
	WinCount(ctx context.Context, begin, end chain.Epoch) ([]*MinerWinCount, error)
	// Traces http://192.168.1.57:3000/project/13/interface/api/83
	Traces(ctx context.Context, start, end chain.Epoch) (r []*TraceMessage, err error)
	// MinersBlockReward http://192.168.1.57:3000/project/13/interface/api/99
	MinersBlockReward(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*MinersBlockReward, error)
	// LatestTipset http://192.168.1.57:3000/project/13/interface/api/235
	LatestTipset(ctx context.Context) ([]*Tipset, error)
	// ActorStateEpoch http://192.168.1.57:3000/project/13/interface/api/239
	ActorStateEpoch(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*ActorStateEpoch, error)
	// Tipset http://192.168.1.57:3000/project/13/interface/api/243
	Tipset(ctx context.Context, epoch chain.Epoch) ([]*Tipset, error)
	// MinerInfo http://192.168.1.57:3000/project/13/interface/api/251
	MinerInfo(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*MinerInfo, error)
	// ActorBalance http://192.168.1.57:3000/project/13/interface/api/255
	ActorBalance(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*ActorBalance, error)
	// MinersForOwner http://192.168.1.57:3000/project/13/interface/api/259
	MinersForOwner(ctx context.Context, addr chain.SmartAddress) ([]*MinersOfOwner, error)
	// ActorMessages http://192.168.1.57:3000/project/13/interface/api/263
	ActorMessages(ctx context.Context, addr chain.SmartAddress, filter types.Filters) (*ActorMessagesList, error)
	// TransferMessages http://192.168.1.57:3000/project/13/interface/api/267
	TransferMessages(ctx context.Context, addr chain.SmartAddress, filter types.Filters) (*TransferMessagesList, error)
	// TransferMessagesByEpoch for imtoken
	TransferMessagesByEpoch(ctx context.Context, addr chain.SmartAddress, filter types.Filters) (*TransferMessagesList, error)
	// MessagesForFund for capital
	MessagesForFund(ctx context.Context, start, end chain.Epoch) (result *TransferMessagesList, err error)
	// TimeOfTrace http://192.168.1.57:3000/project/13/interface/api/271
	TimeOfTrace(ctx context.Context, addr chain.SmartAddress, sort int) ([]*TimeOfTrace, error)
	// MinerCreateTime http://192.168.1.57:3000/project/13/interface/api/275
	MinerCreateTime(ctx context.Context, addr chain.SmartAddress, to string, method int) (*TimeOfTrace, error)
	// MinerGasCost http://192.168.1.57:3000/project/13/interface/api/279
	MinerGasCost(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*MinerGasCost, error)
	// TransferLargeAmount http://192.168.1.57:3000/project/13/interface/api/283
	TransferLargeAmount(ctx context.Context, filter types.Filters) (*TransferLargeAmountList, error)
	// DealsList http://192.168.1.57:3000/project/13/interface/api/287
	DealsList(ctx context.Context, filter types.Filters) (*DealsList, error)
	// DealDetails http://192.168.1.57:3000/project/13/interface/api/291
	DealDetails(ctx context.Context, dealID int64) ([]*DealDetail, error)
	// BlockHeader http://192.168.1.57:3000/project/13/interface/api/295
	BlockHeader(ctx context.Context, filters types.Filters) ([]*BlockHeader, error)
	// ChildTransfersForMessage http://192.168.1.57:3000/project/13/interface/api/299
	ChildTransfersForMessage(ctx context.Context, cid string) ([]*MessageTrace, error)
	// ParentTipset http://192.168.1.57:3000/project/13/interface/api/323
	ParentTipset(ctx context.Context, start chain.Epoch) ([]*ParentTipset, error)
	// BlockHeaderByCid http://192.168.1.57:3000/project/13/interface/api/327
	BlockHeaderByCid(ctx context.Context, cid string) ([]*BlockHeader, error)
	// BlockMessagesByMethodName http://192.168.1.57:3000/project/13/interface/api/343
	BlockMessagesByMethodName(ctx context.Context, filters types.Filters) (*MessagesByMethodNameList, error)
	// ActorMessagesByMethodName http://192.168.1.57:3000/project/13/interface/api/347
	ActorMessagesByMethodName(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (*MessagesByMethodNameList, error)
	// BlockHeadersByMiner http://192.168.1.57:3000/project/13/interface/api/351
	BlockHeadersByMiner(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (*BlockHeadersByMiner, error)
	// DealsByAddr http://192.168.1.57:3000/project/13/interface/api/355
	DealsByAddr(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (*DealsByAddr, error)
	// AllMethods http://192.168.1.57:3000/project/13/interface/api/359
	AllMethods(ctx context.Context) ([]*MethodName, error)
	// AllMethodsForActor http://192.168.1.57:3000/project/13/interface/api/363
	AllMethodsForActor(ctx context.Context, addr chain.SmartAddress) ([]*MethodName, error)
	// BlocksForMessage http://192.168.1.57:3000/project/13/interface/api/367
	BlocksForMessage(ctx context.Context, cid string) ([]*BlockHeader, error)
	// MessagesForBlock http://192.168.1.57:3000/project/13/interface/api/371
	MessagesForBlock(ctx context.Context, cid string, filters types.Filters) ([]*MessageTrace, error)
	// TraceForMessage http://192.168.1.57:3000/project/13/interface/api/399
	TraceForMessage(ctx context.Context, cid string) ([]*MessageTrace, error)
	// BatchTraceForMessage http://192.168.1.57:3000/project/13/interface/api/423
	BatchTraceForMessage(ctx context.Context, start chain.Epoch, cids []string) ([]*MessageTrace, error)
	// RichList http://192.168.1.57:3000/project/13/interface/api/427
	RichList(ctx context.Context, filters types.Filters) (result *RichList, err error)
	// AllMethodsForBlockMessage http://192.168.1.57:3000/project/13/interface/api/431
	AllMethodsForBlockMessage(ctx context.Context, cid string) ([]*MethodName, error)
	// MessagesForBlockByMethodName http://192.168.1.57:3000/project/13/interface/api/435
	MessagesForBlockByMethodName(ctx context.Context, cid string, filters types.Filters) (*MessagesOfBlock, error)
	// DealByID http://192.168.1.57:3000/project/13/interface/api/447
	DealByID(ctx context.Context, dealID int64) ([]*Deals, error)
	// CreateTime http://192.168.1.57:3000/project/13/interface/api/275
	CreateTime(ctx context.Context, addr chain.SmartAddress) (epoch chain.Epoch, err error)
	// CountOfBlockMessages http://192.168.1.57:3000/project/13/interface/api/467
	CountOfBlockMessages(ctx context.Context, start, end chain.Epoch) (count int64, err error)
	// GetTransactionByCid http://192.168.1.57:3000/project/13/interface/api/495
	GetTransactionByCid(ctx context.Context, cid string) (tx *EthTransaction, err error)
	// GetTransactionReceiptByCid http://192.168.1.57:3000/project/13/interface/api/499
	GetTransactionReceiptByCid(ctx context.Context, cid string) (receipt *EthReceipt, err error)
	// GetEvmInitCodeByActorID http://192.168.1.57:3000/project/13/interface/api/503
	GetEvmInitCodeByActorID(ctx context.Context, actorId chain.SmartAddress) (res *ActorInitCode, err error)
	// MessageCidByHash http://192.168.1.57:3000/project/13/interface/api/507
	MessageCidByHash(ctx context.Context, hash string) (*CidOrHash, error)
	// HashByMessageCid http://192.168.1.57:3000/project/13/interface/api/523
	HashByMessageCid(ctx context.Context, messageCid string) (*CidOrHash, error)
	// ChildCallsForMessage http://192.168.1.57:3000/project/13/interface/api/539
	ChildCallsForMessage(ctx context.Context, cid string) (res []*InternalTransfer, err error)
	// InitCodeForEvm http://192.168.1.57:3000/project/13/interface/api/551
	InitCodeForEvm(ctx context.Context, addr chain.SmartAddress) (*InitCode, error)
	// EventsForActor http://192.168.1.57:3000/project/13/interface/api/623
	EventsForActor(ctx context.Context, addr chain.SmartAddress, index int64, limit int64) (result *EventList, err error)
	TestContractTransfer(ctx context.Context, start chain.Epoch, end chain.Epoch) (result []*TraceMessage, err error)
	// TipsetsList http://192.168.1.57:3000/project/13/interface/api/1008
	TipsetsList(ctx context.Context, filters types.Filters) (result *TipsetsList, err error)
	// use for capital info
	GetFilsupply(ctx context.Context, epochs []chain.Epoch) (result []*CirculatingSupply, err error)
	// MessagePool http://192.168.1.57:3000/project/13/interface/api/1110
	MessagePool(ctx context.Context, cid string, filters *types.Filters) (result *MessagePool, err error)
	// AllMethodsForMessagePool http://192.168.1.57:3000/project/13/interface/api/1113
	AllMethodsForMessagePool(ctx context.Context) ([]*MethodName, error)
	// BlockMessageForEpochRange http://192.168.1.57:3000/project/13/interface/api/1116
	BlockMessageForEpochRange(ctx context.Context, start, end chain.Epoch) ([]*BlockMessageCids, error)
	IncomingBlockHeader(ctx context.Context, filters types.Filters) ([]*BlockHeader, error)
	IncomingBlockHeaderByCid(ctx context.Context, cid string) ([]*BlockHeader, error)
	Tipsets(ctx context.Context, filters types.Filters) (result []*Tipset, err error)
	ChangedActors(ctx context.Context, epoch chain.Epoch) (reply []*ChangedActorRes, err error)
}

type ChangedActorRes struct {
	ActorID string
	Code    string
	Balance string
	Epoch   int64
}
type SubtypeData struct {
	Subtype int     `json:"Subtype,omitempty"`
	Data    string  `json:"Data,omitempty"`
	Binary  *Binary `json:"$binary,omitempty"`
}

type Binary struct {
	Base64  string `json:"base64"`
	SubType string `json:"subType"`
}

type Address struct {
	ActorID          string `json:"ActorID"`
	RobustAddress    string `json:"RobustAddress"`
	DelegatedAddress string `json:"DelegatedAddress"`
}

type AggPreNetFee struct {
	Miner       chain.SmartAddress `json:"Miner"`
	Epoch       int64              `json:"Epoch"`
	SectorCount int64              `json:"SectorCount"`
	SignedCid   string             `json:"SignedCid"`
	MethodName  string             `json:"MethodName"`
	BaseFee     decimal.Decimal    `json:"BaseFee"`
	AggFee      decimal.Decimal    `json:"AggFee"`
	BlockTime   int64              `json:"BlockTime"`
}

type AggProNetFee struct {
	Cid         string             `json:"Cid"`
	Epoch       int64              `json:"Epoch"`
	AggFee      decimal.Decimal    `json:"AggFee"`
	MethodName  string             `json:"MethodName"`
	Miner       chain.SmartAddress `json:"Miner"`
	SectorCount int64              `json:"SectorCount"`
	BaseFee     decimal.Decimal    `json:"BaseFee"`
	BlockTime   int64              `json:"BlockTime"`
}

type BlockMessage struct {
	From       chain.SmartAddress `json:"From"`
	To         chain.SmartAddress `json:"To"`
	Method     string             `json:"Method"`
	Value      decimal.Decimal    `json:"Value"`
	SignedCid  string             `json:"SignedCid"`
	GasUsed    decimal.Decimal    `json:"GasUsed"`
	BlockTime  time.Time          `json:"BlockTime"`
	Epoch      int64              `json:"Epoch"`
	ExitCode   int                `json:"ExitCode"`
	Nonce      int                `json:"Nonce"`
	Params     *interface{}       `json:"Params"`
	Return     *interface{}       `json:"Return"`
	GasLimit   int64              `json:"GasLimit"`
	GasPremium decimal.Decimal    `json:"GasPremium"`
	GasFeeCap  decimal.Decimal    `json:"GasFeeCap"`
	Version    int                `json:"Version"`
	GasCost    *GasCost           `json:"GasCost"`
}

type BlockMessagesList struct {
	TotalCount    int64           `json:"TotalCount"`
	BlockMessages []*BlockMessage `json:"BlockMessages"`
}

type GasCost struct {
	BaseFeeBurn        decimal.Decimal `json:"BaseFeeBurn"`
	GasUsed            decimal.Decimal `json:"GasUsed"`
	Message            string          `json:"Message"`
	MinerPenalty       decimal.Decimal `json:"MinerPenalty"`
	MinerTip           decimal.Decimal `json:"MinerTip"`
	OverEstimationBurn decimal.Decimal `json:"OverEstimationBurn"`
	Refund             decimal.Decimal `json:"Refund"`
	TotalCost          decimal.Decimal `json:"TotalCost"`
}

type MinerBlockReward struct {
	Id               int64           `json:"_id"`
	TotalBlockReward decimal.Decimal `json:"TotalBlockReward"`
	BlockCount       int64           `json:"BlockCount"`
}

type MinersBlockReward struct {
	Id               EpochMiner      `json:"_id"`
	TotalBlockReward decimal.Decimal `json:"TotalBlockReward"`
	BlockCount       int64           `json:"BlockCount"`
}

type MinerWinCount struct {
	Id             string          `json:"_id"`
	TotalWinCount  int64           `json:"TotalWinCount"`
	TotalGasReward decimal.Decimal `json:"TotalGasReward"`
}

type EpochMiner struct {
	Epoch int64  `json:"Epoch"`
	Miner string `json:"Miner"`
}

type Tipset struct {
	ID           int64           `json:"_id"`
	Cids         []string        `json:"Cids"`
	MinTimestamp int64           `json:"MinTimestamp"`
	ChildEpoch   int64           `json:"ChildEpoch"`
	State        string          `json:"State"`
	Receipts     string          `json:"Receipts"`
	Weight       string          `json:"Weight"`
	BaseFee      decimal.Decimal `json:"BaseFee"`
}

type TipsetsList struct {
	TotalCount int64    `json:"totalCount"`
	TipSets    []Tipset `json:"tipSets"`
}

type CirculatingSupply struct {
	Id                int64 `json:"_id"`
	CirculatingSupply struct {
		FilVested           string `json:"FilVested"`
		FilMined            string `json:"FilMined"`
		FilBurnt            string `json:"FilBurnt"`
		FilLocked           string `json:"FilLocked"`
		FilCirculating      string `json:"FilCirculating"`
		FilReserveDisbursed string `json:"FilReserveDisbursed"`
	} `json:"CirculatingSupply"`
}

type ActorStateEpoch struct {
	Actor   string          `json:"Addr"`
	Code    string          `json:"Code"`
	Balance decimal.Decimal `json:"Balance"`
	Epoch   int64           `json:"Epoch"`
	Detail  interface{}     `json:"Detail"`
}

// 参照filecoin-project/go-state-types@v0.12.8/manifest/manifest.go
/* 与adapter接口保持一致:
storagepower   power
storagemarket  market
verifiedregistry  verify
storageminer miner
paymentchannel paych
*/
func (actor *ActorStateEpoch) ActorType() string {
	strs := strings.Split(actor.Actor, "/")
	if len(strs) < 2 {
		return ""
	}
	t := strs[len(strs)-1]

	if t == "storagepower" {
		return "power"
	}
	if t == "storagemarket" {
		return "market"
	}
	if t == "verifiedregistry" {
		return "verify"
	}
	if t == "storageminer" {
		return "miner"
	}
	if t == "paymentchannel" {
		return "paych"
	}
	return t
}

type ActorStateDetail struct {
	TotalRawBytePower         string `json:"TotalRawBytePower"`
	TotalBytesCommitted       string `json:"TotalBytesCommitted"`
	TotalQualityAdjPower      string `json:"TotalQualityAdjPower"`
	TotalQABytesCommitted     string `json:"TotalQABytesCommitted"`
	TotalPledgeCollateral     string `json:"TotalPledgeCollateral"`
	ThisEpochRawBytePower     string `json:"ThisEpochRawBytePower"`
	ThisEpochQualityAdjPower  string `json:"ThisEpochQualityAdjPower"`
	ThisEpochPledgeCollateral string `json:"ThisEpochPledgeCollateral"`
	ThisEpochQAPowerSmoothed  struct {
		PositionEstimate string `json:"PositionEstimate"`
		VelocityEstimate string `json:"VelocityEstimate"`
	} `json:"ThisEpochQAPowerSmoothed"`
	MinerCount              int64       `json:"MinerCount"`
	MinerAboveMinPowerCount int64       `json:"MinerAboveMinPowerCount"`
	CronEventQueue          string      `json:"CronEventQueue"`
	FirstCronEpoch          int64       `json:"FirstCronEpoch"`
	Claims                  string      `json:"Claims"`
	ProofValidationBatch    interface{} `json:"ProofValidationBatch"`
}

type PowerActorDetail struct {
	Claims                    interface{}       `json:"Claims"`
	CronEventQueue            interface{}       `json:"CronEventQueue"`
	FirstCronEpoch            int64             `json:"FirstCronEpoch"`
	MinerAboveMinPowerCount   int64             `json:"MinerAboveMinPowerCount"`
	MinerCount                int64             `json:"MinerCount"`
	ProofValidationBatch      interface{}       `json:"ProofValidationBatch"`
	ThisEpochPledgeCollateral decimal.Decimal   `json:"ThisEpochPledgeCollateral"`
	ThisEpochQAPowerSmoothed  ThisEpochSmoothed `json:"ThisEpochQAPowerSmoothed"`
	ThisEpochQualityAdjPower  decimal.Decimal   `json:"ThisEpochQualityAdjPower"`
	ThisEpochRawBytePower     decimal.Decimal   `json:"ThisEpochRawBytePower"`
	TotalBytesCommitted       decimal.Decimal   `json:"TotalBytesCommitted"`
	TotalPledgeCollateral     decimal.Decimal   `json:"TotalPledgeCollateral"`
	TotalQABytesCommitted     decimal.Decimal   `json:"TotalQABytesCommitted"`
	TotalQualityAdjPower      decimal.Decimal   `json:"TotalQualityAdjPower"`
	TotalRawBytePower         decimal.Decimal   `json:"TotalRawBytePower"`
}

type RewardActorDetail struct {
	BaselineTotal           decimal.Decimal   `json:"BaselineTotal"`
	CumsumBaseline          decimal.Decimal   `json:"CumsumBaseline"`
	CumsumRealized          decimal.Decimal   `json:"CumsumRealized"`
	EffectiveBaselinePower  decimal.Decimal   `json:"EffectiveBaselinePower"`
	EffectiveNetworkTime    int64             `json:"EffectiveNetworkTime"`
	Epoch                   int64             `json:"Epoch"`
	SimpleTotal             decimal.Decimal   `json:"SimpleTotal"`
	ThisEpochBaselinePower  decimal.Decimal   `json:"ThisEpochBaselinePower"`
	ThisEpochReward         decimal.Decimal   `json:"ThisEpochReward"`
	ThisEpochRewardSmoothed ThisEpochSmoothed `json:"ThisEpochRewardSmoothed"`
	TotalStoragePowerReward decimal.Decimal   `json:"TotalStoragePowerReward"`
}

type ThisEpochSmoothed struct {
	PositionEstimate decimal.Decimal `json:"PositionEstimate"`
	VelocityEstimate decimal.Decimal `json:"VelocityEstimate"`
}

type MinerInfo struct {
	ID                   string               `json:"ID"`
	Epoch                int64                `json:"Epoch"`
	Miner                chain.SmartAddress   `json:"Miner"`
	Owner                chain.SmartAddress   `json:"Owner"`
	Worker               chain.SmartAddress   `json:"Worker"`
	ControlAddresses     []chain.SmartAddress `json:"ControlAddresses"`
	RawBytePower         decimal.Decimal      `json:"RawBytePower"`
	QualityAdjPower      decimal.Decimal      `json:"QualityAdjPower"`
	Balance              decimal.Decimal      `json:"Balance"`
	AvailableBalance     decimal.Decimal      `json:"AvailableBalance"`
	VestingFunds         decimal.Decimal      `json:"VestingFunds"`
	FeeDebt              decimal.Decimal      `json:"FeeDebt"`
	SectorSize           int64                `json:"SectorSize"`
	SectorCount          int64                `json:"SectorCount"`
	FaultSectorCount     int64                `json:"FaultSectorCount"`
	ActiveSectorCount    int64                `json:"ActiveSectorCount"`
	LiveSectorSector     int64                `json:"LiveSectorSector"`
	RecoverSectorCount   int64                `json:"RecoverSectorCount"`
	TerminateSectorCount int64                `json:"TerminateSectorCount"`
	PreCommitSectorCount int64                `json:"PreCommitSectorCount"`
	InitialPledge        decimal.Decimal      `json:"InitialPledge"`
	PreCommitDeposits    decimal.Decimal      `json:"PreCommitDeposits"`
	State                MinerState           `json:"State"`
	Beneficiary          chain.SmartAddress   `json:"Beneficiary"`
	PeerID               *SubtypeData         `json:"PeerID"`
	Multiaddrs           []*Multiaddrs
}

type Multiaddrs struct {
	Subtype int    `json:"Subtype"`
	Data    string `json:"Data"`
}

type MinerState struct {
	AllocatedSectors           string          `json:"AllocatedSectors"`
	CurrentDeadline            int64           `json:"CurrentDeadline"`
	DeadlineCronActive         bool            `json:"DeadlineCronActive"`
	Deadlines                  string          `json:"Deadlines"`
	EarlyTerminations          *SubtypeData    `json:"EarlyTerminations"`
	FeeDebt                    decimal.Decimal `json:"FeeDebt"`
	Info                       string          `json:"Info"`
	InitialPledge              decimal.Decimal `json:"InitialPledge"`
	LockedFunds                decimal.Decimal `json:"LockedFunds"`
	PreCommitDeposits          string          `json:"PreCommitDeposits"`
	PreCommittedSectors        string          `json:"PreCommittedSectors"`
	PreCommittedSectorsCleanUp string          `json:"PreCommittedSectorsCleanUp"`
	ProvingPeriodStart         int64           `json:"ProvingPeriodStart"`
	Sectors                    string          `json:"Sectors"`
	VestingFunds               string          `json:"VestingFunds"`
}

type TimeOfTrace struct {
	Epoch int64 `json:"Epoch"`
}

type MinerGasCost struct {
	ID      chain.SmartAddress `json:"_id"`
	GasCost decimal.Decimal    `json:"GasCost"`
}

type ActorBalance struct {
	Actor   chain.SmartAddress `json:"Actor"`
	Epoch   int64              `json:"Epoch"`
	Balance decimal.Decimal    `json:"Balance"`
	Code    string             `json:"Code"`
}

type MinersOfOwner struct {
	Owner  string   `json:"Owner"`
	Miners []string `json:"Miners"`
}

type ActorMessages struct {
	Cid       string             `json:"Cid,omitempty"`
	SignedCid string             `json:"SignedCid,omitempty"`
	RootCid   string             `json:"RootCid,omitempty"`
	Epoch     int64              `json:"Epoch"`
	From      chain.SmartAddress `json:"From"`
	To        chain.SmartAddress `json:"To"`
	Value     decimal.Decimal    `json:"Value"`
	ExitCode  int64              `json:"ExitCode,omitempty"`
	Method    string             `json:"Method"`
	Depth     int64              `json:"Depth"`
}

type ActorMessagesList struct {
	TotalCount    int64            `json:"TotalCount"`
	ActorMessages []*ActorMessages `json:"MessagesForActor"`
}

type MessagesByMethodNameList struct {
	TotalCount           int64            `json:"TotalCount"`
	MessagesByMethodName []*ActorMessages `json:"MessagesByMethodName"`
}

type TransferLargeAmountList struct {
	TotalCount          int64            `json:"TotalCount"`
	TransferLargeAmount []*ActorMessages `json:"TransferMessagesForLargeAmount"`
}

type TransferMessagesList struct {
	TotalCount       int64            `json:"totalCount"`
	TransferMessages []*ActorMessages `json:"transferMessages"`
}

type MinerGasCostSize struct {
	MinerGasCost32G []decimal.Decimal   `json:"MinerGasCost32G"`
	AvgGasCost32G   decimal.NullDecimal `json:"AvgGasCost32G"`
	MinerGasCost64G []decimal.Decimal   `json:"MinerGasCost64G"`
	AvgGasCost64G   decimal.NullDecimal `json:"AvgGasCost64G"`
}

type Deals struct {
	ID                   int64              `json:"_id"`
	Epoch                int64              `json:"Epoch"`
	PieceCID             string             `json:"PieceCID"`
	PieceSize            int64              `json:"PieceSize"`
	VerifiedDeal         bool               `json:"VerifiedDeal"`
	Client               chain.SmartAddress `json:"Client"`
	Provider             chain.SmartAddress `json:"Provider"`
	StartEpoch           int64              `json:"StartEpoch"`
	EndEpoch             int64              `json:"EndEpoch"`
	StoragePricePerEpoch decimal.Decimal    `json:"StoragePricePerEpoch"`
	ProviderCollateral   decimal.Decimal    `json:"ProviderCollateral"`
	ClientCollateral     decimal.Decimal    `json:"ClientCollateral"`
	Label                interface{}        `json:"Label"`
}

type DealsList struct {
	TotalCount int64    `json:"TotalCount"`
	DealsList  []*Deals `json:"deals"`
}

type DealDetail struct {
	DealID               int64              `json:"DealID"`
	Epoch                int64              `json:"Epoch"`
	Cid                  string             `json:"Cid"`
	PieceCID             string             `json:"PieceCID"`
	VerifiedDeal         bool               `json:"VerifiedDeal"`
	Client               chain.SmartAddress `json:"Client"`
	Provider             chain.SmartAddress `json:"Provider"`
	ProviderCollateral   decimal.Decimal    `json:"ProviderCollateral"`
	ClientCollateral     decimal.Decimal    `json:"ClientCollateral"`
	StartEpoch           int64              `json:"StartEpoch"`
	EndEpoch             int64              `json:"EndEpoch"`
	PieceSize            int64              `json:"PieceSize"`
	StoragePricePerEpoch decimal.Decimal    `json:"StoragePricePerEpoch"`
}

type BlockHeaderList struct {
	TotalCount   int64          `json:"TotalCount"`
	BlockHeaders []*BlockHeader `json:"BlockHeaders"`
}

type BlockHeader struct {
	ID            string        `json:"_id"`
	Miner         string        `json:"Miner"`
	Epoch         int64         `json:"Epoch"`
	ElectionProof ElectionProof `json:"ElectionProof"`
	Ticket        Ticket        `json:"Ticket"`
	MessageCount  int64         `json:"MessageCount"`
	Timestamp     int64
	Parents       []string
	FirstSeen     int64
	ParentWeight  string
}

type ElectionProof struct {
	VRFProof *SubtypeData `json:"VRFProof"`
	WinCount int64        `json:"WinCount"`
}

type Ticket struct {
	VRFProof *SubtypeData `json:"VRFProof"`
}

type MessageTrace struct {
	ID           int64              `json:"_id,omitempty"`
	Epoch        chain.Epoch        `json:"Epoch,omitempty"`
	Cid          string             `json:"Cid"`
	Value        decimal.Decimal    `json:"Value"`
	From         chain.SmartAddress `json:"From"`
	To           chain.SmartAddress `json:"To"`
	ExitCode     int                `json:"ExitCode"`
	Method       string             `json:"Method"`
	TransferList []*TransferInfo    `json:"TransferList,omitempty"`
	//Params       *SubtypeData       `json:"Params"`
	Params interface{} `json:"Params"`
	//Return       *SubtypeData       `json:"Return"`
	Return       interface{}     `json:"Return"`
	ParamsDetail interface{}     `json:"ParamsDetail"`
	ReturnDetail interface{}     `json:"ReturnDetail"`
	Version      int             `json:"Version"`
	Nonce        int64           `json:"Nonce"`
	GasLimit     int64           `json:"GasLimit"`
	GasFeeCap    decimal.Decimal `json:"GasFeeCap"`
	GasPremium   decimal.Decimal `json:"GasPremium"`
	GasCost      *GasCost        `json:"GasCost,omitempty"`
	Error        string          `json:"Error"`
}

type InternalTransfer struct {
	ID           int64              `json:"_id,omitempty"`
	Epoch        chain.Epoch        `json:"Epoch,omitempty"`
	Cid          string             `json:"Cid"`
	Value        decimal.Decimal    `json:"Value"`
	From         chain.SmartAddress `json:"From"`
	To           chain.SmartAddress `json:"To"`
	ExitCode     int                `json:"ExitCode"`
	Method       string             `json:"Method"`
	InnerCalls   []*InnerCall       `json:"InnerCalls,omitempty"`
	Params       *SubtypeData       `json:"Params"`
	Return       *SubtypeData       `json:"Return"`
	ParamsDetail interface{}        `json:"ParamsDetail"`
	ReturnDetail interface{}        `json:"ReturnDetail"`
	Version      int                `json:"Version"`
	Nonce        int64              `json:"Nonce"`
	GasLimit     int64              `json:"GasLimit"`
	GasFeeCap    decimal.Decimal    `json:"GasFeeCap"`
	GasPremium   decimal.Decimal    `json:"GasPremium"`
	GasCost      *GasCost           `json:"GasCost,omitempty"`
	Error        string             `json:"Error"`
}

type InnerCall struct {
	To         chain.SmartAddress `json:"To"`
	From       chain.SmartAddress `json:"From"`
	Value      string             `json:"Value"`
	MethodName string             `json:"MethodName"`
}

type TransferInfo struct {
	ID         string             `json:"_id"`
	Version    int                `json:"Version"`
	To         chain.SmartAddress `json:"To"`
	From       chain.SmartAddress `json:"From"`
	Nonce      int                `json:"Nonce"`
	Value      decimal.Decimal    `json:"Value"`
	GasLimit   int64              `json:"GasLimit"`
	GasFeeCap  decimal.Decimal    `json:"GasFeeCap"`
	GasPremium decimal.Decimal    `json:"GasPremium"`
	Method     int                `json:"Method"`
	Params     interface{}        `json:"Params"`
	Detail     interface{}        `json:"Detail"`
	SignedCid  string             `json:"SignedCid"`
}

type TransferDetail struct {
	Actor        string      `json:"Actor"`
	Method       string      `json:"Method"`
	PackedHeight int64       `json:"PackedHeight"`
	Params       interface{} `json:"Params"`
}

type TraceParamsDetail struct {
	Method int                `json:"Method"`
	Params interface{}        `json:"Params"`
	To     chain.SmartAddress `json:"To"`
	Value  decimal.Decimal    `json:"Value"`
}

type TraceReturnDetail struct {
	Applied bool        `json:"Applied"`
	Code    int         `json:"Code"`
	Ret     interface{} `json:"Ret"`
	TxnID   int         `json:"TxnID"`
}

type TraceMessage struct {
	ID           string
	Cid          string
	SignedCid    *string
	Epoch        int64
	Seq          []int64
	Depth        int
	Ver          string
	Msg          *Msg
	MsgRct       *MsgRct
	Error        *string
	SeqIndex     [][]int64
	SubCallCount int
	GasCost      *GasCost
	Return       interface{}
	Version      int
	To           chain.SmartAddress
	From         chain.SmartAddress
	Nonce        int64
	Value        decimal.Decimal
	GasLimit     int64
	GasFeeCap    string
	GasPremium   string //
	Method       int64
	Params       interface{}
	Detail       *MessageDetail
	ParamsBson   *primitive.Binary
	Actor        string
	IsBlock      bool
}

type Msg struct {
	From   chain.SmartAddress
	To     chain.SmartAddress
	Method int
}

type MsgRct struct {
	ExitCode int
	GasUsed  int64
	Return   interface{}
}

type MessageDetail struct {
	Actor        string
	Method       string
	PackedHeight int64
	Params       interface{}
}

type ParentTipset struct {
	ID           int64           `json:"_id"`
	Cids         []string        `json:"Cids"`
	MinTimestamp int64           `json:"MinTimestamp"`
	ChildEpoch   int64           `json:"ChildEpoch"`
	State        string          `json:"State"`    // 根
	Receipts     string          `json:"Receipts"` // 执行结果根
	Weight       decimal.Decimal `json:"Weight"`
	BaseFee      decimal.Decimal `json:"BaseFee"`
}

type BlockHeadersByMiner struct {
	TotalCount   int64          `json:"TotalCount"`
	BlockHeaders []*BlockHeader `json:"BlockHeaders"`
}

type DealsByAddr struct {
	TotalCount int64    `json:"TotalCount"`
	DealsList  []*Deals `json:"Deals"`
}

type MethodName struct {
	ID         string `json:"_id,omitempty"`
	MethodName string `json:"MethodName,omitempty"`
	Count      int64  `json:"Count"`
}

type MessagesOfBlock struct {
	TotalCount int64           `json:"TotalCount"`
	Messages   []*MessageTrace `json:"Messages"`
}

type RichInfo struct {
	Addr    chain.SmartAddress `json:"Addr"`
	Balance decimal.Decimal    `json:"Balance"`
	Code    string             `json:"Code"`
}

type RichList struct {
	TotalCount int64       `json:"TotalCount"`
	RichList   []*RichInfo `json:"RichList"`
}

type InitCode struct {
	InitCode string `json:"InitCode"`
}

type CidOrHash struct {
	Cid  string `json:"cid,omitempty"`
	Hash string `json:"hash,omitempty"`
}

type EthTransaction struct {
	ChainId              string        `json:"chainId"`
	Nonce                string        `json:"nonce"`
	Hash                 string        `json:"hash"`
	BlockHash            interface{}   `json:"blockHash"`
	BlockNumber          interface{}   `json:"blockNumber"`
	TransactionIndex     interface{}   `json:"transactionIndex"`
	From                 string        `json:"from"`
	To                   string        `json:"to"`
	Value                string        `json:"value"`
	Type                 string        `json:"type"`
	Input                string        `json:"input"`
	Gas                  string        `json:"gas"`
	MaxFeePerGas         string        `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string        `json:"maxPriorityFeePerGas"`
	AccessList           []interface{} `json:"accessList"`
	V                    string        `json:"v"`
	R                    string        `json:"r"`
	S                    string        `json:"s"`
}

type EthReceipt struct {
	TransactionHash   string           `json:"transactionHash"`
	TransactionIndex  string           `json:"transactionIndex"`
	BlockHash         string           `json:"blockHash"`
	BlockNumber       string           `json:"blockNumber"`
	From              string           `json:"from"`
	To                string           `json:"to"`
	Root              string           `json:"root"`
	Status            string           `json:"status"`
	ContractAddress   interface{}      `json:"contractAddress"`
	CumulativeGasUsed string           `json:"cumulativeGasUsed"`
	GasUsed           string           `json:"gasUsed"`
	EffectiveGasPrice string           `json:"effectiveGasPrice"`
	LogsBloom         string           `json:"logsBloom"`
	Logs              []*EthReceiptLog `json:"logs"`
	Type              string           `json:"type"`
}

type EthReceiptLog struct {
	Address          string   `json:"address"`
	Data             string   `json:"data"`
	Topics           []string `json:"topics"`
	Removed          bool     `json:"removed"`
	LogIndex         string   `json:"logIndex"`
	TransactionIndex string   `json:"transactionIndex"`
	TransactionHash  string   `json:"transactionHash"`
	BlockHash        string   `json:"blockHash"`
	BlockNumber      string   `json:"blockNumber"`
}

type ActorInitCode struct {
	InitCode string `json:"InitCode"`
}

type EventList struct {
	TotalCount     int      `json:"totalCount"`
	EventsForActor []*Event `json:"eventsForActor"`
}

type Event struct {
	ActorID   string   `json:"ActorID"`
	Epoch     int      `json:"Epoch"`
	Cid       string   `json:"Cid"`
	SignedCid string   `json:"SignedCid"`
	Topics    []string `json:"Topics"`
	Data      string   `json:"Data"`
	LogIndex  int      `json:"LogIndex"`
	Removed   bool     `json:"Removed"`
}

type BlockMessageCids struct {
	BlockCid string   `json:"BlockCid"`
	Epoch    int64    `json:"Epoch"`
	Messages []string `json:"Messages"`
}
