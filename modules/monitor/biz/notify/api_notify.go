package notify

type AlertNotify interface {
	Send(receivers []string, template string, content []string) error
	GetDetail() string
}
