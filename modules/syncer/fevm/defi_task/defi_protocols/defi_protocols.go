package defi_protocols

import (
	"encoding/json"
	"fmt"
	"github.com/gozelle/logger"
	
	"io/ioutil"
	"net/http"
	
	"github.com/shopspring/decimal"
	
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
)

var log = logger.NewLogger("defi-protocols")
var OssPrefix = "https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/defi/"

type DefiProtocols interface {
	GetTvl() (Tvl, error)
	GetUsers(repository.ERC20TokenRepo) (int, error)
	GetProtocolName() string
	GetContractId() string
	GetIconUrl() string
}

type Tvl struct {
	Usd decimal.Decimal
	Fil decimal.Decimal
}

//	type TvlData struct {
//		Date int64   `json:"date"`
//		Fil  float64 `json:"FIL"`
//	}
type TvlTokens struct {
	Date   int64 `json:"date"`
	Tokens struct {
		Fil float64 `json:"FIL"`
	} `json:"tokens"`
}

func GetTvlFromDefilama(protocolName string) (Tvl, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.llama.fi/protocol/%s", protocolName))
	if err != nil {
		log.Error("request defilama failed : %w", err)
		return Tvl{}, err
	}
	defer resp.Body.Close()
	
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Tvl{}, err
	}
	
	res := struct {
		TvlDatas []TvlTokens `json:"tokensInUsd"`
		Tokens   []TvlTokens `json:"tokens"`
	}{}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		return Tvl{}, err
	}
	
	if len(res.TvlDatas) == 0 || len(res.Tokens) == 0 {
		return Tvl{}, fmt.Errorf("no valid information in result")
	}
	
	return Tvl{
		Usd: decimal.NewFromFloat(res.TvlDatas[len(res.TvlDatas)-1].Tokens.Fil),
		Fil: decimal.NewFromFloat(res.Tokens[len(res.Tokens)-1].Tokens.Fil),
	}, nil
	
}

func GetTvlFromDefilamaThs() (Tvl, error) {
	resp, err := http.Get("https://api.llama.fi/protocol/themis-pro")
	if err != nil {
		log.Error("request defilama failed : %w", err)
		return Tvl{}, err
	}
	defer resp.Body.Close()
	
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Tvl{}, err
	}
	
	res := struct {
		A struct {
			Filecoin float64 `json:"Filecoin"`
			Staking  float64 `json:"Filecoin-staking"`
		} `json:"currentChainTvls"`
	}{}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		return Tvl{}, err
	}
	
	return Tvl{
		Usd: decimal.NewFromFloat(res.A.Filecoin + res.A.Staking),
		Fil: decimal.Zero,
	}, nil
	
}
