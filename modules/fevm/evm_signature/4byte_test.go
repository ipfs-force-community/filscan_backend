package evm_signature

import (
	"context"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"testing"
	"time"
)

func TestAPI4Byte(t *testing.T) {
	api := NewAPI4Byte("https://www.4byte.directory", resty.New())

	f, err := fs.Lookup("configs/ty.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	signatureDB := dal.NewEvmSignatureDal(db)

	//signatureApi, err := api.EventSignature(context.Background(), nil)
	//require.NoError(t, err)
	//var pageCount int64
	//if signatureApi != nil && signatureApi.Count != 0 {
	//	pageMod := big.Mod(big.NewInt(signatureApi.Count), big.NewInt(100)).Int64()
	//	if pageMod != 0 {
	//		pageCount = (signatureApi.Count / 100) + 1
	//	} else {
	//		pageCount = signatureApi.Count / 100
	//	}
	//}
	//
	//fmt.Printf("total page count: %d", pageCount)

	for page := 245; page <= 1904; page++ {
		var newSignatureApi *EventSignatureList
		newSignatureApi, err = api.EventSignature(context.Background(), page)
		require.NoError(t, err)
		if newSignatureApi != nil {
			convertor := EvmSignatureConvertor{}
			result := convertor.EventSignatureAPIToEventSignatureDB(newSignatureApi.Results)
			err = signatureDB.SaveEvmEventSignatures(context.Background(), result)
			require.NoError(t, err)
			fmt.Printf("page: %d saved! \n", page)
		}
		if page%5 == 0 {
			time.Sleep(5 * time.Second)
		}
	}
}
