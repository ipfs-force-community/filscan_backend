package filscan

import (
	"context"
	
	"github.com/shopspring/decimal"
)

type Empty struct{}

type FNSAPI interface {
	FnsSummary(ctx context.Context, request FNSFnsSummaryRequest) (reply *FnsSummaryReply, err error)
	FnsTransfers(ctx context.Context, request *FnsTransfersRequest) (reply *FnsTransfersReply, err error)
	FnsControllers(ctx context.Context, request *FnsOwnersRequest) (reply *FnsOwnersReply, err error)
	FnsDomainDetail(ctx context.Context, request *FnsDetailRequest) (reply *FnsDetailReply, err error)
	FnsAddressDomains(ctx context.Context, request *FnsAddressDomainsRequest) (reply *FnsControllerDomainsReply, err error)
	FnsBindDomains(ctx context.Context, request *FnsBindDomainsRequest) (reply FnsBindDomainsReply, err error)
}

type FNSFnsSummaryRequest struct {
	Provider string `json:"provider"`
}

type FnsSummaryReply struct {
	TotalSupply int64  `json:"total_supply"`
	Owners      int64  `json:"owners"`
	Transfers   int64  `json:"transfers"`
	Contract    string `json:"contract"`
	//Provider    string `json:"provider"`
	TokenName   string `json:"token_name"`
	Logo        string `json:"logo"`
	MainSite    string `json:"main_site"`
	TwitterLink string `json:"twitter_link"`
}

type FnsTransfersRequest struct {
	Provider string `json:"provider"`
	Index    int    `json:"index"`
	Limit    int    `json:"limit"`
}

type FnsTransfersReply struct {
	Total int64          `json:"total"`
	Items []*FnsTransfer `json:"items"`
}

type FnsTransfer struct {
	Cid    string `json:"cid"`
	Method string `json:"method"`
	Time   int64  `json:"time"`
	From   string `json:"from"`
	To     string `json:"to"`
	Item   string `json:"item"`
}

type FnsOwnersRequest struct {
	Provider string `json:"provider"`
	Index    int    `json:"index"`
	Limit    int    `json:"limit"`
}
type FnsOwnersReply struct {
	Total int64       `json:"total"`
	Items []*FnsOwner `json:"items"`
}

type FnsOwner struct {
	Rank       int64           `json:"rank"`
	Controller string          `json:"controller"`
	Amount     int64           `json:"amount"`
	Percentage decimal.Decimal `json:"percentage"`
}

type FnsDetailRequest struct {
	Provider string `json:"provider"`
	Domain   string `json:"domain"`
}
type FnsDetailReply struct {
	ResolvedAddress string `json:"resolved_address"`
	ExpiredAt       int64  `json:"expired_at"`
	Registrant      string `json:"registrant"`
	Controller      string `json:"controller"`
	Exists          bool   `json:"exists"`
	IconUrl         string `json:"icon_url"`
}

type FnsAddressDomainsRequest struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type FnsControllerDomainsReply struct {
	Registrant      string                 `json:"registrant"`
	ResolvedAddress string                 `json:"resolvedAddress"`
	Domains         []*FnsDomainsReplyItem `json:"domains"`
}

type FnsDomainsReplyItem struct {
	Domain   string `json:"domain"`
	Provider string `json:"provider"`
	LOGO     string `json:"logo"`
	Name     string `json:"name"`
}

type FnsBindDomainsRequest struct {
	Addresses []string
}

type FnsBindDomainsReply struct {
	Provider string            `json:"provider"`
	Domains  map[string]string `json:"domains"`
}
