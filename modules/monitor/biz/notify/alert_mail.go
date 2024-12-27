package notify

import (
	"strings"
	"sync"
	"time"

	"github.com/gozelle/mail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

type AlertMail struct {
	conf    *config.Config
	mClient *mail.Client
	lock    *sync.Mutex
}

func NewAlertMail(conf *config.Config, m *mail.Client) *AlertMail {
	return &AlertMail{
		conf:    conf,
		mClient: m,
		lock:    new(sync.Mutex),
	}
}

func initMsg() (*mail.Msg, error) {
	msg := mail.NewMsg()
	err := msg.From("FilscanTeam<admin@filscan.io>")
	if err != nil {
		return nil, err
	}
	msg.Subject("节点告警")
	return msg, nil
}

func (m *AlertMail) Send(receivers []string, template string, content []string) error {
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
	}()
	msg, err := initMsg()
	msg.Subject(content[0])
	if err != nil {
		return err
	}
	for _, receiver := range receivers {
		err = msg.To(receiver)
		if err != nil {
			return err
		}
		templateMsg := strings.ReplaceAll(strTpl, "$code", template)
		if content[0] == "会员到期提醒" {
			templateMsg = strings.ReplaceAll(templateMsg, "$honorific", "")
		} else {
			templateMsg = strings.ReplaceAll(templateMsg, "$honorific", "祝商祺！")
		}
		templateMsg = strings.ReplaceAll(templateMsg, "$time", time.Now().Format("2006-01-02"))
		msg.SetBodyString(mail.TypeTextHTML, templateMsg)
		err = m.mClient.DialAndSend(msg)
		if err != nil {
			return err
		}
		logger.Infof("AlertMail send msg [%s] success to %s", template, receiver)
	}
	return nil
}

func (m *AlertMail) GetDetail() string {
	return "AlertMail"
}
