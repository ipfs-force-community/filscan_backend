package filscan

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type ResourceAPI interface {
	GetBannerList(ctx context.Context, req *GetBannerListReq) (*GetBannerListReply, error)

	GetFEvmCategory(ctx context.Context, _ struct{}) (*GetFEvmCategoryReply, error)
	GetFEvmItemsByCategory(ctx context.Context, req *GetFEvmItemsByCategoryReq) (*GetFEvmItemsByCategoryReply, error)
	GetFEvmHotItems(ctx context.Context, _ struct{}) (*GetFEvmItemsByCategoryReply, error)

	GetEventsList(ctx context.Context, _ struct{}) (*GetEventsListReply, error)

	GetFilecoinKLine(ctx context.Context, req *GetFilecoinKLineReq) (*GetFilecoinKLineRes, error)
	GetFilecoinChange(ctx context.Context, req *GetFilecoinChangeReq) (*GetFilecoinChangeRes, error)
	GetFilecoinTrend(ctx context.Context, req *GetFilecoinTrendReq) (*GetFilecoinTrendRes, error)
	TokenHolderAddress(ctx context.Context, _ struct{}) (*TokenHolderAddressRes, error)
	TopActiveAddress(ctx context.Context, _ struct{}) (*[]Flows, error)
	TokenHolderTrend(ctx context.Context, req *TokenHolderTrendReq) (*[]TokenHolderTrendRes, error)
	FilecoinBaseData(ctx context.Context, _ struct{}) (*FilecoinBaseDataReply, error)
	NetworkCapital(ctx context.Context, _ struct{}) (*NetworkCapitalReply, error)
	NetworkCapitalFigure(ctx context.Context, req *NetworkCapitalFigureReq) (*NetworkCapitalFigureReply, error)
	VestReleaseDate(ctx context.Context, _ struct{}) (*ReleaseDate, error)
}

type FilecoinBaseDataReply struct {
	Locked            float64 `json:"locked"`
	ChangedAmount     float64 `json:"changed_amount"`
	Vol               float64 `json:"vol"`
	TxsRate           float64 `json:"txs_rate"`
	CirculatingRate   float64 `json:"circulating_rate"`
	Burn              float64 `json:"burn"`
	LockedRate        float64 `json:"locked_rate"`
	MaxSupply         float64 `json:"max_supply"`
	BurnRate          float64 `json:"burn_rate"`
	Circulating       float64 `json:"circulating"`
	ChangedVol        float64 `json:"changed_vol"`
	ChangeRate        float64 `json:"change_rate"`
	CirculatingAmount float64 `json:"circulating_amount"`
	Price             float64 `json:"price"`
	PriceChangeRate   float64 `json:"price_change_rate"`
	Rank              int     `json:"rank"`
	RmbPrice          float64 `json:"rmb_price"`
}

type NetworkCapitalReply struct {
	FilProduce     decimal.Decimal `json:"fil_produce"`
	FilProduce24h  decimal.Decimal `json:"fil_produce_24h"`
	Mined          decimal.Decimal `json:"mined"`
	Mined24h       decimal.Decimal `json:"mined_24h"`
	Vested         decimal.Decimal `json:"vested"`
	Vested24h      decimal.Decimal `json:"vested_24h"`
	Reserved       decimal.Decimal `json:"reserved"`
	Reserved24h    decimal.Decimal `json:"reserved_24h"`
	Locked         decimal.Decimal `json:"locked"`
	Locked24h      decimal.Decimal `json:"locked_24h"`
	Pledge         decimal.Decimal `json:"pledge"`
	Pledge24h      decimal.Decimal `json:"pledge_24h"`
	DefiTvl        decimal.Decimal `json:"defi_tvl"`
	DefiTvl24h     decimal.Decimal `json:"defi_tvl_24h"`
	Burn           decimal.Decimal `json:"burn"`
	Burn24h        decimal.Decimal `json:"burn_24h"`
	Circulation    decimal.Decimal `json:"circulation"`
	Circulation24h decimal.Decimal `json:"circulation_24h"`
}

type NetworkCapitalFigureReq struct {
	Interval string `json:"interval"`
}

type NetworkCapitalFigureReply struct {
	Epoch     int64                  `json:"epoch"`
	BlockTime int64                  `json:"block_time"`
	List      []*NetworkCapitalPoint `json:"list"`
}

type NetworkCapitalPoint struct {
	Epoch           int64           `json:"epoch"`
	BlockTime       int64           `json:"block_time"`
	Circulating     decimal.Decimal `json:"circulating"`
	Produced        decimal.Decimal `json:"produced"`
	Locked          decimal.Decimal `json:"locked"`
	Burn            decimal.Decimal `json:"burn"`
	Price           float64         `json:"price"`
	PriceChangeRate float64         `json:"price_change_rate"`
	Rank            int             `json:"rank"`
	RmbPrice        float64         `json:"rmb_price"`
}

type ReleaseItem struct {
	AccountTag      string  `json:"account_tag"`
	Released        float64 `json:"released"`
	UnlockStartTime int64   `json:"unlock_start_time"`
	AccountID       string  `json:"account_id"`
	Balance         float64 `json:"balance"`
	UnlockEndTime   int64   `json:"unlock_end_time"`
	InitialBalance  float64 `json:"initial_balance"`
	BalanceChanged  float64 `json:"balance_changed"`
}

type ReleaseDate struct {
	ReleaseItemList []ReleaseItem `json:"account_list"`
}

type TokenHolderTrendReq struct {
	Interval string `json:"interval"`
}

type TokenHolderAddressRes struct {
	AddrCount  int64   `json:"addrcount"`
	Top10Rate  float64 `json:"top10rate"`
	Top20Rate  float64 `json:"top20rate"`
	Top50Rate  float64 `json:"top50rate"`
	Top100Rate float64 `json:"top100rate"`
}

type TokenHolderTrendRes struct {
	Timpstamp  int     `json:"timpstamp"`
	Addrcount  int     `json:"addrcount"`
	Top10Rate  float64 `json:"top10rate"`
	Top20Rate  float64 `json:"top20rate"`
	Top50Rate  float64 `json:"top50rate"`
	Top100Rate float64 `json:"top100rate"`
}

type Flows struct {
	Address      string  `json:"address"`
	Quantity     float64 `json:"quantity"`
	Percentage   float64 `json:"percentage"`
	Platform     string  `json:"platform"`
	PlatformName string  `json:"platform_name"`
	Logo         string  `json:"logo"`
	Change       float64 `json:"change"`
	Blockurl     string  `json:"blockurl"`
	ChangeAbs    float64 `json:"change_abs"`
	Updatetime   string  `json:"updatetime"`
	Hidden       int     `json:"hidden"`
	Destroy      int     `json:"destroy"`
	Iscontract   int     `json:"iscontract"`
	Addressflag  string  `json:"addressflag"`
}

type GetFilecoinTrendReq struct {
	Code string `json:"code"`
	Webp int64  `json:"webp"`
	Type string `json:"type"`
}

type GetFilecoinTrendRes = GetFilecoinKLineRes

type GetFilecoinChangeReq struct {
	Code string `json:"code"`
	Webp int64  `json:"webp"`
}

type GetFilecoinChangeRes = GetFilecoinKLineRes

type GetFilecoinKLineReq struct {
	TikckerId string `json:"tickerid"`
	Period    int64  `json:"period"`
	Reach     int64  `json:"reach"`
	Since     string `json:"since"`
	Utc       int64  `json:"utc"`
	Webp      int64  `json:"webp"`
}

type GetFilecoinKLineRes struct {
	Data string `json:"data"`
}

type GetBannerListReq struct {
	Category string `json:"category"`
	Language string `json:"language"`
}

type BannerItem struct {
	Url  string `json:"url"`
	Link string `json:"link"`
}

type GetBannerListReply struct {
	Items []BannerItem `json:"items"`
}

type GetFEvmCategoryItem struct {
	Label string `json:"label"`
	Num   int    `json:"num"`
}

type GetFEvmCategoryReply = []GetFEvmCategoryItem
type GetFEvmItemsByCategoryReq struct {
	Category string `json:"category"`
}

type FEvmItem struct {
	Twitter  string `json:"twitter"`
	MainSite string `json:"main_site"`
	Name     string `json:"name"`
	Logo     string `json:"logo"`
	Category string `json:"detail"`
	Orders   int    `json:"-"`
}

type GetFEvmItemsByCategoryReply = []FEvmItem

type Events struct {
	ImageUrl string    `json:"image_url"`
	JumpUrl  string    `json:"jump_url"`
	StartAt  time.Time `json:"start_at"`
	EndAt    time.Time `json:"end_at"`
	Name     string    `json:"name"`
}

type GetEventsListReply struct {
	Items []Events `json:"items"`
}
