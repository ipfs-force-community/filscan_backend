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
