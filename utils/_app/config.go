package _app

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
)

func PrintConfig(value interface{}) {
	// TODO 在生产环境不打印
	d, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		log.Errorf("打印配置文件错误: %s", err)
		return
	}
	log.Infof("配置文件: \n%s", string(d))
}

type DingTalk struct {
	AgentID   int64  `json:"agent_id" mapstructure:"agent_id"`
	AppKey    string `json:"app_key" mapstructure:"app_key"`
	AppSecret string `json:"app_secret" mapstructure:"app_secret"`
}

type Postgres struct {
	Host           string  `json:"host" mapstructure:"host"`
	User           string  `json:"user" mapstructure:"user"`
	Password       string  `json:"password" mapstructure:"password"`
	Database       string  `json:"database" mapstructure:"database"`
	Port           *int    `json:"port,omitempty" mapstructure:"port"`
	ConnectTimeout *int    `json:"connect_timeout,omitempty" mapstructure:"connect_timeout"`
	SSLMode        *string `json:"ssl_mode,omitempty" mapstructure:"ssl_mode"`
	Timezone       *string `json:"timezone,omitempty" mapstructure:"timezone"`
}

func (p *Postgres) GetConnectTimeout() int {
	if p.ConnectTimeout != nil {
		return *p.ConnectTimeout
	}
	return 100
}

func (p *Postgres) GetSSLMode() string {
	if p.SSLMode != nil {
		return *p.SSLMode
	}
	return "disable"
}

func (p *Postgres) GetTimezone() string {
	if p.Timezone != nil {
		return *p.Timezone
	}
	return "Asia/Shanghai"
}

func (p *Postgres) GetPort() int {
	if p.Port != nil {
		return *p.Port
	}
	return 5432
}

func (p *Postgres) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?connect_timeout=%d&sslmode=%s&TimeZone=%s",
		p.User,
		p.Password,
		p.Host,
		p.GetPort(),
		p.Database,
		p.GetConnectTimeout(),
		p.GetSSLMode(),
		p.GetTimezone(),
	)
}

func InitConfig(config string) {
	viper.SetConfigType("toml")
	viper.SetConfigFile(config)
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
	}
}

type Config struct {
	LaniakeaDSN string
}
