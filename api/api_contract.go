package filscan

import (
	"context"

	"github.com/shopspring/decimal"
)

type ContractAPI interface {
	VerifyContractAPI
	EvmContractAPI
}

type VerifyContractAPI interface {
	VerifyContract(ctx context.Context, request VerifyContractRequest) (resp VerifyContractResponse, err error)
	SolidityVersions(ctx context.Context, request VersionListRequest) (resp VersionListResponse, err error)
	Licenses(ctx context.Context, request VersionListRequest) (resp VersionListResponse, err error)
	VerifiedContractList(ctx context.Context, request VerifiedContractListRequest) (resp VerifiedContractListResponse, err error)
	VerifiedContractByActorID(ctx context.Context, request VerifiedContractRequest) (resp VerifiedContractResponse, err error)
	VerifyHardhatContract(ctx context.Context, request VerifyHardhatContractRequest) (resp VerifyContractResponse, err error)
}

type EvmContractAPI interface {
	EvmContractList(ctx context.Context, request EvmContractRequest) (resp EvmContractResponse, err error)
	ActorEventsList(ctx context.Context, req ActorEventsListRequest) (resp ActorEventsListResponse, err error)
	EvmContractSummary(ctx context.Context, req struct{}) (resp EvmContractSummaryResponse, err error)
	EvmTxsHistory(ctx context.Context, req EvmTxsHistoryReq) (resp EvmTxsHistoryRes, err error)
	EvmGasTrend(ctx context.Context, req EvmGasTrendReq) (resp EvmGasTrendRes, err error)
}

type EvmTxsHistoryReq struct {
	Interval string `json:"interval"` // 时间间隔
}

type EvmGasTrendReq struct {
	Interval string `json:"interval"` // 时间间隔
}

type EvmTxs struct {
	Timestamp int64 `json:"timestamp"`
	TxsCount  int64 `json:"txs_count"`
}

type EvmTxsHistoryRes struct {
	EvmTxsHistory []EvmTxs `json:"points"`
}

type EvmGas struct {
	Timestamp int64           `json:"timestamp"`
	TxsGas    decimal.Decimal `json:"txs_gas"`
}

type EvmGasTrendRes struct {
	Epoch       int64
	BlockTime   int64
	EvmGasTrend []EvmGas `json:"points"`
}

type VerifyHardhatContractRequest struct {
	ContractAddress      string       `json:"contract_address"`        // 合约地址
	SourceFile           []SourceFile `json:"source_file"`             // 源文件
	CompileVersion       string       `json:"compile_version"`         // 编译器版本
	Optimize             bool         `json:"optimize"`                // 优化选项
	OptimizeRuns         int64        `json:"optimize_runs"`           // 优化参数（默认为200）
	Arguments            string       `json:"arguments"`               // 合约参数
	License              string       `json:"license"`                 // 证书
	MateDataFile         *SourceFile  `json:"mate_data_file"`          // 元数据文件
	HardhatBuildInfoFile *SourceFile  `json:"hardhat_build_info_file"` // hardhat的build-info文件
}

type EvmContractSummaryResponse struct {
	TotalContract            int64 `json:"total_contract"`
	TotalContractChangeIn24h int64 `json:"total_contract_change_in_24h"`
	ContractTxs              int64 `json:"contract_txs"`
	ContractTxsChangeIn24h   int64 `json:"contract_txs_change_in_24h"`
	ContractUsers            int64 `json:"contract_users"`
	ContractUsersChangeIn24h int64 `json:"contract_users_change_in_24h"`
	VerifiedContracts        int64 `json:"verified_contracts"`
}

type VerifyContractRequest struct {
	ContractAddress      string       `json:"contract_address"`        // 合约地址
	SourceFile           []SourceFile `json:"source_file"`             // 源文件
	CompileVersion       string       `json:"compile_version"`         // 编译器版本
	Optimize             bool         `json:"optimize"`                // 优化选项
	OptimizeRuns         int64        `json:"optimize_runs"`           // 优化参数（默认为200）
	Arguments            string       `json:"arguments"`               // 合约参数
	License              string       `json:"license"`                 // 证书
	MateDataFile         *SourceFile  `json:"mate_data_file"`          // 元数据文件
	HardhatBuildInfoFile *SourceFile  `json:"hardhat_build_info_file"` // hardhat的build-info文件
}

type VerifyContractResponse struct {
	IsVerified   bool          `json:"is_verified"`
	CompiledFile *CompiledFile `json:"compiled_file"`
}

type VersionListRequest struct {
}

type VersionListResponse struct {
	VersionList []string `json:"version_list"`
}

type VerifiedContractRequest struct {
	InputAddress string `json:"input_address"`
}

type VerifiedContractResponse struct {
	CompiledFile *CompiledFile `json:"compiled_file"`
	SourceFile   []*SourceFile `json:"source_file"`
}

type VerifiedContractListRequest struct {
	Index *int `json:"index"`
	Limit *int `json:"limit"`
}

type VerifiedContractListResponse struct {
	CompiledFileList []*CompiledFile `json:"compiled_file_list"`
	Total            int64           `json:"total"`
}

type EvmContractRequest struct {
	Page     int    `json:"page"`
	Limit    int    `json:"limit"`
	Interval string `json:"interval"`
	Field    string `json:"field"`
	Sort     string `json:"sort"`
}

type EvmContractResponse struct {
	EvmContractList []*EvmContract `json:"evm_contract_list"`
	Total           int64          `json:"total"`
	UpdateTime      int64          `json:"update_time"`
}

type ActorEventsListRequest struct {
	ActorID string `json:"actor_id"`
	Page    int64  `json:"page"`
	Limit   int64  `json:"limit"`
}

type ActorEventsListResponse struct {
	EventList  []*Event `json:"event_list"`
	TotalCount int64    `json:"total_count"`
}

type SourceFile struct {
	FileName   string `json:"file_name"`   // 文件名
	SourceCode string `json:"source_code"` // 源文件代码
}

type CompiledFile struct {
	ActorID         string `json:"actor_id"`                    // actorID
	ActorAddress    string `json:"actor_address"`               // actorAddress
	ContractAddress string `json:"contract_address"`            // 合约地址
	License         string `json:"license"`                     // 证书
	Language        string `json:"language"`                    // 编译语言
	Compiler        string `json:"compiler"`                    // 编译器版本
	Optimize        bool   `json:"optimize"`                    // 优化选项
	OptimizeRuns    int64  `json:"optimize_runs"`               // 优化参数,默认为200
	ContractName    string `json:"contract_name"`               // 合约名称
	Arguments       string `json:"arguments,omitempty"`         // 合约构造参数
	ByteCode        string `json:"byte_code,omitempty"`         // 字节码
	ABI             string `json:"ABI,omitempty"`               // ABI
	LocalByteCode   string `json:"local_byte_code,omitempty"`   // 本地编译字节码
	HasBeenVerified bool   `json:"has_been_verified,omitempty"` // 是否已经被验证过
}

type EvmContract struct {
	Rank            int              `json:"rank,omitempty"`
	ActorID         string           `json:"actor_id"`                   // actorID
	ActorAddress    string           `json:"actor_address,omitempty"`    // actorAddress
	ContractAddress string           `json:"contract_address,omitempty"` // ETH合约地址
	ContractName    string           `json:"contract_name"`              // 合约名称
	TransferCount   int64            `json:"transfer_count"`             // 交易数量
	UserCount       int64            `json:"user_count"`                 // 用户数量
	ActorBalance    *decimal.Decimal `json:"actor_balance,omitempty"`    // actor余额
	GasCost         decimal.Decimal  `json:"gas_cost"`                   // gas消耗
}

type Event struct {
	ActorID   string   `json:"actor_id"`
	Epoch     int      `json:"epoch"`
	Cid       string   `json:"cid"`
	EventName string   `json:"event_name"`
	Topics    []string `json:"topics"`
	Data      string   `json:"data"`
	LogIndex  int      `json:"log_index"`
	Removed   bool     `json:"removed"`
}
