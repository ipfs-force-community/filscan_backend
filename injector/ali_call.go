package injector

import (
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dyvmsapi20170525 "github.com/alibabacloud-go/dyvmsapi-20170525/v3/client"
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

var ALiCallClientSet = wire.NewSet(NewAliCallClient)

func NewAliCallClient(conf *config.Config) (_result *dyvmsapi20170525.Client, err error) {
	apiConf := &openapi.Config{
		AccessKeyId:     conf.ALi.AccessKeyId,
		AccessKeySecret: conf.ALi.AccessKeySecret,
	}
	// Endpoint 请参考 https://api.aliyun.com/product/Dyvmsapi
	apiConf.Endpoint = conf.ALi.CallClient
	_result = &dyvmsapi20170525.Client{}
	_result, err = dyvmsapi20170525.NewClient(apiConf)

	return _result, err
}
