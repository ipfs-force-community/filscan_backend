package browser

import (
	logging "github.com/gozelle/logger"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

var log = logging.NewLogger("biz")

func NewBrowserBiz(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB, abiDecoder fevm.ABIDecoderAPI, conf *config.Config) *BrowserBiz {
	// 三个统计端点的读路径开关（[feature] 段，默认全关 = 原样透传，行为与改造前一致）。
	// 只包 API 侧：同步器用的 agg 不受影响（见 agg_pg_reward.go 文件头）。
	agg = NewPgRewardAgg(agg, db, conf)
	// 大额转账列表端点的读路径开关（同上，默认关闭；口径与空页形态见 agg_pg_large_amount.go）。
	agg = NewPgLargeAmountAgg(agg, db, conf)
	return &BrowserBiz{
		IndexBiz:          NewIndexBiz(agg, adapter, db, conf),
		BlockChainBiz:     NewBlockChainBiz(agg, adapter, db, conf),
		AccountBiz:        NewAccountBiz(agg, adapter, db, conf),
		RankBiz:           NewRankBiz(db, conf),
		StatisticBiz:      NewStatisticBiz(db, adapter, conf),
		FnsBiz:            NewFnsBiz(db, abiDecoder),
		VerifyContractBiz: NewVerifyContract(agg, adapter, db, conf),
		ContractBiz:       NewContract(db, agg, adapter, conf),
		ERC20Biz:          NewERC20Biz(db, adapter, agg, abiDecoder),
		NFTBiz:            NewNFTBiz(db),
		DefiDashboardBiz:  NewDefiDashboardBiz(agg, adapter, db),
		IMTokenBiz:        NewIMTokenBiz(agg, adapter, db, conf),
		ResourceBiz:       NewResourceBiz(db, adapter, agg),
	}
}

var _ filscan.BrowserAPI = (*BrowserBiz)(nil)

type BrowserBiz struct {
	*IndexBiz
	*BlockChainBiz
	*AccountBiz
	*RankBiz
	*StatisticBiz
	*FnsBiz
	*VerifyContractBiz
	*ContractBiz
	*ERC20Biz
	*NFTBiz
	*DefiDashboardBiz
	*IMTokenBiz
	*ResourceBiz
}
