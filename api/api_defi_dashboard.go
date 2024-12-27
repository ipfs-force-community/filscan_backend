package filscan

import (
	"context"

	"github.com/shopspring/decimal"
)

type DefiDashboardAPI interface {
	DefiSummary(ctx context.Context, _ struct{}) (*DefiSummaryReply, error)
	DefiProtocolList(ctx context.Context, req DefiDashboardListRequest) (*DefiDashboardListResponse, error)
}

type DefiSummaryReply struct {
	FevmStaked        decimal.Decimal `json:"fevm_staked"`
	StakedChangeIn24h decimal.Decimal `json:"staked_change_in_24h"`
	TotalUser         int             `json:"total_user"`
	UserChangeIn24h   int             `json:"user_change_in_24h"`
	FilStaked         decimal.Decimal `json:"fil_staked"`
	UpdatedAt         int64           `json:"updated_at"`
}

type DefiDashboardListRequest struct {
	Page    int    `json:"page"`
	Limit   int    `json:"limit"`
	Reverse bool   `json:"reverse"`
	Field   string `json:"field"`
}

type DefiItems struct {
	Protocol           string          `json:"protocol"`
	Tvl                decimal.Decimal `json:"tvl"`
	TvlChangeRateIn24h decimal.Decimal `json:"tvl_change_rate_in_24h"`
	TvlChangeIn24h     decimal.Decimal `json:"tvl_change_in_24h"`
	Users              int             `json:"users"`
	IconUrl            string          `json:"icon_url"`
	Tokens             []StakedToken   `json:"tokens"`
	MainSite           string          `json:"main_site"`
}

type StakedToken struct {
	TokenName string  `json:"token_name"`
	IconUrl   string  `json:"icon_url"`
	Rate      float64 `json:"rate"`
}

type DefiDashboardListResponse struct {
	Total int         `json:"total"`
	Items []DefiItems `json:"items"`
}
