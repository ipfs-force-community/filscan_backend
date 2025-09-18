package _ave

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gozelle/logger"

	"github.com/shopspring/decimal"
	"golang.org/x/sync/singleflight"
)

var log = logger.NewLogger("ave")
var singleFlight = singleflight.Group{}

type ExchangeInfo struct {
	LatestPrice decimal.Decimal
	Vol24       decimal.Decimal
}

type timerCache struct {
	mutex sync.RWMutex
	mp    map[string]*ExchangeInfo
}

func (t *timerCache) Get(contractID string) *ExchangeInfo {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	val := t.mp[contractID]
	if val != nil {
		tmp := *val
		return &tmp
	}
	return nil
}

func (t *timerCache) Set(contactID string, exchange ExchangeInfo, cacheTime time.Duration) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.mp[contactID] = &exchange
}

var tc timerCache

func init() {
	tc = timerCache{
		mutex: sync.RWMutex{},
		mp:    map[string]*ExchangeInfo{},
	}
}

type geckoSimplePriceResp struct {
	Data struct {
		Attributes struct {
			TokenPrices map[string]string `json:"token_prices"`
		} `json:"attributes"`
	} `json:"data"`
}

type geckoPoolAttributes struct {
	Name              string `json:"name"`
	BaseTokenPriceUsd string `json:"base_token_price_usd"`
	VolumeUsd         struct {
		H24 float64 `json:"h24"`
	} `json:"volume_usd"`
}

type geckoTokenPoolsResp struct {
	Data []struct {
		Attributes geckoPoolAttributes `json:"attributes"`
	} `json:"data"`
}

func fetchTokenPrice(contractID string) (price, vol24 decimal.Decimal) {
	client := &http.Client{Timeout: 10 * time.Second}

	priceUrl := fmt.Sprintf("https://api.geckoterminal.com/api/v2/simple/networks/filecoin/token_price/%s", contractID)
	req, _ := http.NewRequest("GET", priceUrl, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var r geckoSimplePriceResp
		if err := json.Unmarshal(body, &r); err == nil {
			if p, ok := r.Data.Attributes.TokenPrices[contractID]; ok {
				price, _ = decimal.NewFromString(p)
			}
		}
	}

	poolsUrl := fmt.Sprintf("https://api.geckoterminal.com/api/v2/networks/filecoin/tokens/%s/pools", contractID)
	req, _ = http.NewRequest("GET", poolsUrl, nil)
	req.Header.Set("Accept", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		if price.IsZero() {
			log.Errorf("请求 token %s 池子失败: %s", contractID, err)
		}
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var r geckoTokenPoolsResp
	err = json.Unmarshal(body, &r)
	if err != nil {
		return
	}
	for _, pool := range r.Data {
		v := decimal.NewFromFloat(pool.Attributes.VolumeUsd.H24)
		vol24 = vol24.Add(v)
		if price.IsZero() {
			p, _ := decimal.NewFromString(pool.Attributes.BaseTokenPriceUsd)
			if p.GreaterThan(price) {
				price = p
			}
		}
	}
	return
}

func RefreshToken(contractID string, td time.Duration) {
	time.Sleep(td)
	go RefreshToken(contractID, td)

	price, vol24 := fetchTokenPrice(contractID)
	tc.Set(contractID, ExchangeInfo{
		LatestPrice: price,
		Vol24:       vol24,
	}, 30*time.Minute)
}

func GetTokenExchangeInfo(contractID string) ExchangeInfo {
	v, _, _ := singleFlight.Do(contractID, func() (interface{}, error) {
		res := tc.Get(contractID)
		if res == nil {
			price, vol24 := fetchTokenPrice(contractID)
			res = &ExchangeInfo{
				LatestPrice: price,
				Vol24:       vol24,
			}
			tc.Set(contractID, *res, 5*time.Minute)
			go RefreshToken(contractID, time.Minute*30)
		}
		return *res, nil
	})
	return v.(ExchangeInfo)
}
