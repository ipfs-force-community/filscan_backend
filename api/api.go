package filscan

const (
	BrowserNamespace = "filscan"
)

type BrowserAPI interface {
	IndexAPI
	BlockChainAPI
	RankAPI
	StatisticAPI
	FNSAPI
	ContractAPI
	ERC20API
	DefiDashboardAPI
	NFTAPI
	ResourceAPI
}
