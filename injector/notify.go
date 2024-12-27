package injector

import (
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	dyvmsapi20170525 "github.com/alibabacloud-go/dyvmsapi-20170525/v3/client"
	"github.com/google/wire"
	"github.com/gozelle/mail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/notify"
)

var NotifySet = wire.NewSet(NewNotify)

func NewNotify(conf *config.Config, msgClient *dysmsapi20170525.Client, callClient *dyvmsapi20170525.Client, m *mail.Client) *notify.Notify {
	return notify.NewNotify(conf, msgClient, callClient, m)
}
