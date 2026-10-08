package v19

import (
	"github.com/filecoin-project/go-state-types/builtin/v19/market"
)

// AddVerifiedClientParams

type AddVerifiedClientParams struct {
	Address   string
	Allowance string
}

// ApproveReturn

type ApproveReturn struct {
	Applied bool
	Code    string
	Ret     string
}

// Return

type Return struct {
	ActorID       int64
	RobustAddress string
	EthAddress    string
}

// ExecReturn

type ExecReturn struct {
	IDAddress     string
	RobustAddress string
}

// GetAllowanceParams

type GetAllowanceParams struct {
	Owner    string
	Operator string
}

// TxnIDParams

type TxnIDParams struct {
	ID           int64
	ProposalHash string
}

// ChangeBeneficiaryParams

type ChangeBeneficiaryParams struct {
	NewBeneficiary string
	NewQuota       int64
	NewExpiration  string
}

// GetBeneficiaryReturn

type BeneficiaryTerm struct {
	Quota      string
	UsedQuota  string
	Expiration string
}

type PendingBeneficiaryChange struct {
	NewBeneficiary        string
	NewQuota              string
	NewExpiration         string
	ApprovedByBeneficiary bool
	ApprovedByNominee     bool
}

type ActiveBeneficiary struct {
	Beneficiary string
	Term        BeneficiaryTerm
}

type GetBeneficiaryReturn struct {
	Active   ActiveBeneficiary
	Proposed *PendingBeneficiaryChange
}

// MaxTerminationFeeParams

type MaxTerminationFeeParams struct {
	Power         string
	InitialPledge string
}

// ChangeMultiaddrsParams

type ChangeMultiaddrsParams struct {
	NewMultiaddrs []string
}

// ChangeWorkerAddressParams

type ChangeWorkerAddressParams struct {
	NewWorker       string
	NewControlAddrs []string
}

// ChangePeerIDParams

type ChangePeerIDParams struct {
	NewID string
}

// CompactPartitionsParams

type CompactPartitionsParams struct {
	Deadline   int64
	Partitions string
}

// CompactSectorNumbersParams

type CompactSectorNumbersParams struct {
	MaskSectorNumbers string
}

// CreateMinerParams

type CreateMinerParams struct {
	Owner               string
	Worker              string
	WindowPoStProofType int64
	Peer                string
	Multiaddrs          []string
}

// CreateMinerReturn

type CreateMinerReturn struct {
	IDAddress     string
	RobustAddress string
}

// DeclareFaultsRecoveredParams

type DeclareFaultsRecoveredParams struct {
	Recoveries []RecoveryDeclaration
}

// DisputeWindowedPoStParams

type DisputeWindowedPoStParams struct {
	Deadline  int64
	PoStIndex int64
}

// ExecParams

type ExecParams struct {
	CodeCID           string
	ConstructorParams string
}

type RecoveryDeclaration struct {
	Deadline  int64
	Partition int64
	Sectors   string
}

// ExtendClaimTermsParams

type ExtendClaimTermsParams struct {
	Terms []ClaimTerm
}

type ClaimTerm struct {
	Provider int64
	ClaimId  int64
	TermMax  int64
}

// ExtendClaimTermsReturn

type ExtendClaimTermsReturn BatchReturn

// ExtendSectorExpirationParams

type ExtendSectorExpirationParams struct {
	Extensions []ExpirationExtension
}

type ExpirationExtension struct {
	Deadline      int64
	Partition     int64
	Sectors       string
	NewExpiration int64
}

// ExtendSectorExpiration2Params

type ExtendSectorExpiration2Params struct {
	Extensions []ExpirationExtension2
}

type ExpirationExtension2 struct {
	Deadline          int64
	Partition         int64
	Sectors           string
	SectorsWithClaims []SectorClaim
	NewExpiration     int64
}

type SectorClaim struct {
	SectorNumber   int64
	MaintainClaims []int64
	DropClaims     []int64
}

// IncreaseAllowanceParams

type IncreaseAllowanceParams struct {
	Operator string
	Increase string
}

// PreCommitSectorParams

type PreCommitSectorParams struct {
	SealProof              int64
	SectorNumber           int64
	SealedCID              string
	SealRandEpoch          int64
	DealIDs                []int64
	Expiration             int64
	ReplaceCapacity        bool
	ReplaceSectorDeadline  int64
	ReplaceSectorPartition int64
	ReplaceSectorNumber    int64
}

// PreCommitSectorBatchParams

type PreCommitSectorBatchParams struct {
	Sectors []PreCommitSectorParams
}

// SectorPreCommitInfo

type SectorPreCommitInfo struct {
	SealProof     int64
	SectorNumber  int64
	SealedCID     string
	SealRandEpoch int64
	DealIDs       []int64
	Expiration    int64
	UnsealedCid   *string
}

type PreCommitSectorBatchParams2 struct {
	Sectors []SectorPreCommitInfo
}

// ProposeParams

type ProposeParams struct {
	To     string
	Value  string
	Method string
	Params string
}

// ProveCommitAggregateParams

type ProveCommitAggregateParams struct {
	SectorNumbers  string
	AggregateProof string
}

// ProveCommitSectorParams

type ProveCommitSectorParams struct {
	SectorNumber string
	Proof        string
}

// ProveReplicaUpdatesParams

type ProveReplicaUpdatesParams struct {
	Updates []ReplicaUpdate
}

type ReplicaUpdate struct {
	SectorID           int64
	Deadline           int64
	Partition          int64
	NewSealedSectorCID string
	Deals              []int64
	UpdateProofType    int64
	ReplicaProof       string
}

// ProposeReturn

type ProposeReturn struct {
	TxnID   int64
	Applied bool
	Code    string
	Ret     string
}

//PublishStorageDealsParams

type PublishStorageDealsParams struct {
	Deals []ClientDealProposal
}

type ClientDealProposal struct {
	Proposal        DealProposal
	ClientSignature Signature
}

type DealProposal struct {
	PieceCID             string
	PieceSize            int64
	VerifiedDeal         bool
	Client               string
	Provider             string
	Label                market.DealLabel
	StartEpoch           string
	EndEpoch             string
	StoragePricePerEpoch string
	ProviderCollateral   string
	ClientCollateral     string
}

type Signature struct {
	Type byte
	Data string
}

// PublishStorageDealsReturn

type PublishStorageDealsReturn struct {
	IDs        []int64
	ValidDeals string
}

// RemoveExpiredAllocationsParams

type RemoveExpiredAllocationsParams struct {
	Client        string
	AllocationIds []int64
}

// ReportConsensusFaultParams

type ReportConsensusFaultParams struct {
	BlockHeader1     string
	BlockHeader2     string
	BlockHeaderExtra string
}

// RemoveExpiredAllocationsReturn

type RemoveExpiredAllocationsReturn struct {
	Considered       []int64
	Results          BatchReturn
	DataCapRecovered string
}

type BatchReturn struct {
	SuccessCount int64
	FailCodes    []FailCode
}

type FailCode struct {
	Idx  int64
	Code int64
}

// SubmitWindowedPoStParams

type SubmitWindowedPoStParams struct {
	Deadline         int64
	Partitions       []PoStPartition
	Proofs           []PoStProof
	ChainCommitEpoch string
	ChainCommitRand  string
}

type PoStPartition struct {
	Index   int64
	Skipped string
}

type PoStProof struct {
	PoStProof  int64
	ProofBytes string
}

// TerminateSectorsParams

type TerminateSectorsParams struct {
	Terminations []TerminationDeclaration
}

type TerminationDeclaration struct {
	Deadline  int64
	Partition int64
	Sectors   string
}

// TerminateSectorsReturn

type TerminateSectorsReturn struct {
	Done bool
}

// TransferFromParams

type TransferFromParams struct {
	From         string
	To           string
	Amount       string
	OperatorData string
}

// TransferFromReturn

type TransferFromReturn struct {
	FromBalance   string
	ToBalance     string
	Allowance     string
	RecipientData string
}

// WithdrawBalanceParams(market)

type WithdrawBalanceParamsMarket struct {
	ProviderOrClientAddress string
	Amount                  string
}

// WithdrawBalanceParams(miner)

type WithdrawBalanceParamsMiner struct {
	AmountRequested string
}

// UpgradeSectorQualityParams (FIP-0118, NV29 新增的 miner method 37)

type UpgradeSectorQualityParams struct {
	Upgrades []UpgradeSectorQuality
}

type UpgradeSectorQuality struct {
	Deadline      int64
	Partition     int64
	Sectors       string
	NewExpiration *int64
}

// ===== NV29(Solstice / FIP-0118) 奖励流方法（f02 的 9 个 FRC 方法）=====
//
// 类型与字段名以 go-state-types v0.19.0 builtin/v19/reward（reward_types.go / streams.go）为准。
// 对外一律把地址/金额转成字符串、把枚举转成整数，与既有 converter 风格一致。

// WeightRecord 是权重记录（Denom=1e18 定点）。
type WeightRecord struct {
	VStart uint64
	Slope  int64
	TStart int64
	Floor  uint64
	Cap    uint64
}

// WeightRecordUpdate 是 SetWeightRecords / StepWeightRecords 里对一条流的权重替换。
type WeightRecordUpdate struct {
	ID     uint64
	Weight WeightRecord
}

type SetWeightRecordsParams struct {
	Updates []WeightRecordUpdate
}

type StepWeightRecordsParams struct {
	Updates []WeightRecordUpdate
}

// RecipientShare 是显式流份额表里的一项（Share 为 Denom 定点）。
type RecipientShare struct {
	Recipient string
	Share     uint64
}

// DistributionInit 是显式流的初始分配（Writer + 完整份额表）。
type DistributionInit struct {
	Writer string
	Shares []RecipientShare
}

// RegisterStreamParams 注册一条流；Distribution 为 nil 表示隐式流（矿工共识流）。
type RegisterStreamParams struct {
	ID              uint64
	Weight          WeightRecord
	Distribution    *DistributionInit
	ActivationEpoch int64
}

type RemoveStreamParams struct {
	ID uint64
}

type SetDistributionParams struct {
	ID     uint64
	Writer string
}

type SetSharesParams struct {
	ID     uint64
	Shares []RecipientShare
}

type ReplaceAddressParams struct {
	ID         uint64
	OldAddress string
	NewAddress string
}

// CancelPendingParams 取消一条排队写入；ID 为 nil 表示日程级（SetWeightRecords）。
type CancelPendingParams struct {
	ID *uint64
	Op uint8
}

// ClaimParams 提取请求：流 ID + 钱包列表（按序对应 ClaimReturn.Amounts）。
type ClaimParams struct {
	ID      uint64
	Wallets []string
}

// ClaimReturn 提取返回：与 ClaimParams.Wallets 等长、按序对应的金额（attoFIL，无法支付的为 "0"）。
type ClaimReturn struct {
	Amounts []string
}
