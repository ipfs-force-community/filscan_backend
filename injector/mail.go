package injector

import (
	"github.com/google/wire"
	"github.com/gozelle/mail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

var MailClientSet = wire.NewSet(NewMailClient)

func NewMailClient(conf *config.Config) (*mail.Client, error) {
	return mail.NewClient(*conf.Mail.Client,
		mail.WithPort(*conf.Mail.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(*conf.Mail.Username),
		mail.WithPassword(*conf.Mail.Password),
	)
}
