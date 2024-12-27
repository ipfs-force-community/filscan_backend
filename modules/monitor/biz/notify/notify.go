package notify

import (
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	dyvmsapi20170525 "github.com/alibabacloud-go/dyvmsapi-20170525/v3/client"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/mail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

var logger = logging.NewLogger("notify")

// 在这里生成三个全局的对象吧，然后发送地址啥的还是和规则
type Notify struct {
	AlertMail
	AlertALiCall
	AlertALiMsg
}

var GlobalNotify *Notify

func NewNotify(conf *config.Config, msgClient *dysmsapi20170525.Client, callClient *dyvmsapi20170525.Client, m *mail.Client) *Notify {
	GlobalNotify = &Notify{
		AlertMail:    *NewAlertMail(conf, m),
		AlertALiCall: *NewAlertALiCall(conf, callClient),
		AlertALiMsg:  *NewAlertALiMsg(conf, msgClient),
	}
	return GlobalNotify
}
