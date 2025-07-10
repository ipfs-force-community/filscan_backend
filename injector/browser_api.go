package injector

import (
	"github.com/google/wire"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	biz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/biz/browser"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/typer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

var APIProvider = wire.NewSet(NewBrowserAPI)

func NewBrowserAPI(conf *config.Config, agg londobell.Agg, adapter londobell.Adapter, typer *typer.Typer, db *gorm.DB, decoder fevm.ABIDecoderAPI) filscan.BrowserAPI {
	//if conf.MockMode {
	//	return browser.NewBrowserAPI()
	//}
	return biz.NewBrowserBiz(agg, adapter, db, decoder, conf)
}

//func NewCronAPI(conf *config.Config) filscan.CronAPI {
//	if conf.MockMode {
//		return mock.NewCronAPI()
//	}
//	return biz.NewCronBiz()
//}
