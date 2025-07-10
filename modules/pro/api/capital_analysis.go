package pro

import (
	"context"

	"github.com/shopspring/decimal"
)

type CapitalAnalysisAPI interface {
	CapitalAddrInfo(ctx context.Context, req AddrInfoRequest) (resp AddrInfoResp, err error)
	CapitalAddrTransaction(ctx context.Context, req AddrTransactionRequest) (resp AddrTransactionResp, err error)
	EvaluateAddr(ctx context.Context, req EvaluateAddrRequest) (resp EvaluateAddrResp, err error)
}

type EvaluateAddrRequest struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

type EvaluateAddrResp struct {
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
}

type Node struct {
	Address                         string          `json:"address"`
	ShortAddress                    string          `json:"short_address"`
	TotalTransactionVolume          decimal.Decimal `json:"total_transaction_volume"`
	ToTransactionVolume             decimal.Decimal `json:"to_transaction_volume"`
	FromTransactionVolume           decimal.Decimal `json:"from_transaction_volume"`
	TransactionVolumeWithFatherNode decimal.Decimal `json:"transaction_volume_with_father_node"`
	TotalCnt                        int64           `json:"total_count"`
	ToCnt                           int64           `json:"to_cnt"`
	FromCnt                         int64           `json:"from_cnt"`
	UniqueToTransactionAddressCnt   int64           `json:"unique_to_transaction_address_cnt"`
	UniqueFromTransactionAddressCnt int64           `json:"unique_from_transaction_address_cnt"`
	CntWithFatherNode               int64           `json:"cnt_with_father_node"`
	ProportionWithFatherNode        decimal.Decimal `json:"proportion_with_father_node"`
	CalChildTransactionVolume       decimal.Decimal `json:"cal_child_transaction_volume"`
	CalChildCnt                     int64           `json:"cal_child_cnt"`
	Width                           int64           `json:"width"`
	Level                           int64           `json:"level"`
	Tag                             string          `json:"tag"`
	Nodes                           []*Node         `json:"nodes,omitempty"`
}

type Edge struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	TotalVal decimal.Decimal `json:"total_val"`
	//FromVal  decimal.Decimal `json:"from_val"`
	//ToVal    decimal.Decimal `json:"to_val"`
	TotalCnt int64 `json:"total_cnt"`
	//ToCnt    int64           `json:"to_cnt"`
	//FromCnt  int64           `json:"from_cnt"`
	Direction string     `json:"direction"`
	Details   []*SubEdge `json:"details"`
}

type SubEdge struct {
	Cid          string          `json:"cid"`
	Value        decimal.Decimal `json:"value"`
	Epoch        int64           `json:"epoch"`
	ExchangeHour int64           `json:"exchange_hour"`
	Direction    string          `json:"direction"`
}

type AddrInfoRequest struct {
	Address  string `json:"address"`
	Interval string `json:"interval"`
}

type AddrInfoResp struct {
	Tag             string          `json:"tag"`
	Rank            int             `json:"rank"`
	Balance         decimal.Decimal `json:"balance"`
	Proportion      decimal.Decimal `json:"proportion"`
	BalanceIncrease decimal.Decimal `json:"balance_increase"`
}

type AddrTransactionRequest struct {
	Address string `json:"address"`
}

type AddrTransactionResp struct {
	Tag                   string          `json:"tag"`
	Balance               decimal.Decimal `json:"balance"`
	Proportion            decimal.Decimal `json:"proportion"`
	TotalTransactionValue decimal.Decimal `json:"total_transaction_value"`
	TotalTransactionCount int64           `json:"total_transaction_Count"`
}
