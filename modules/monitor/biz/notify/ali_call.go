package notify

import (
	"sync"

	dyvmsapi20170525 "github.com/alibabacloud-go/dyvmsapi-20170525/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

type AlertALiCall struct {
	conf       *config.Config
	callClient *dyvmsapi20170525.Client
	lock       *sync.Mutex
}

func NewAlertALiCall(conf *config.Config, client *dyvmsapi20170525.Client) *AlertALiCall {
	return &AlertALiCall{
		conf:       conf,
		callClient: client,
		lock:       new(sync.Mutex),
	}
}

func initCallReq() *dyvmsapi20170525.SingleCallByTtsRequest {
	return &dyvmsapi20170525.SingleCallByTtsRequest{
		CalledNumber: tea.String(""),
	}
}

func (c *AlertALiCall) Send(receivers []string, template string, content []string) error {
	c.lock.Lock()
	defer func() {
		c.lock.Unlock()
	}()
	req := initCallReq()
	req.TtsCode = &template
	req.TtsParam = tea.String(content[0])
	for _, receiver := range receivers {
		req.CalledNumber = &receiver
		runtime := &util.RuntimeOptions{}
		resp, err := c.callClient.SingleCallByTtsWithOptions(req, runtime)
		if err != nil {
			return err
		}
		logger.Infof(*util.ToJSONString(resp))
		logger.Infoln("AlertALiCall send msg [%s] success to %s", content, receiver)
	}
	return nil
}

func (c *AlertALiCall) GetDetail() string {
	return "AlertALiCall"
}
