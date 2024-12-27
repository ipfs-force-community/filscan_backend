package notify

import (
	"sync"

	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

type AlertALiMsg struct {
	conf      *config.Config
	msgClient *dysmsapi20170525.Client
	lock      *sync.Mutex
}

func NewAlertALiMsg(conf *config.Config, client *dysmsapi20170525.Client) *AlertALiMsg {
	return &AlertALiMsg{
		conf:      conf,
		msgClient: client,
		lock:      new(sync.Mutex),
	}
}

func initMsgReq() *dysmsapi20170525.SendSmsRequest {
	return &dysmsapi20170525.SendSmsRequest{
		SignName: tea.String("Filscan"),
	}
}

func (c *AlertALiMsg) Send(receivers []string, template string, content []string) error {
	c.lock.Lock()
	defer func() {
		c.lock.Unlock()
	}()
	req := initMsgReq()
	req.TemplateCode = &template
	req.TemplateParam = tea.String(content[0])
	for _, receiver := range receivers {
		req.PhoneNumbers = &receiver
		runtime := &util.RuntimeOptions{}
		resp, err := c.msgClient.SendSmsWithOptions(req, runtime)
		if err != nil {
			return err
		}
		logger.Infoln(*util.ToJSONString(resp))
		logger.Infof("AlertALiMsg send msg [%s] success to %s", content, receiver)
	}
	return nil
}

func (c *AlertALiMsg) GetDetail() string {
	return "AlertALiMsg"
}
