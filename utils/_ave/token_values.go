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
	//
	//go func() {
	//	ticker := time.NewTicker(cacheTime)
	//	<-ticker.C
	//	t.mutex.Lock()
	//	defer t.mutex.Unlock()
	//	t.mp[contactID] = nil
	//}()
}

var tc timerCache

func init() {
	tc = timerCache{
		mutex: sync.RWMutex{},
		mp:    map[string]*ExchangeInfo{},
	}
}

type dexscreenerPair struct {
	PriceUsd string `json:"priceUsd"`
	Volume   struct {
		H24 float64 `json:"h24"`
	} `json:"volume"`
}

type dexscreenerResp struct {
	Pairs []dexscreenerPair `json:"pairs"`
}

func fetchTokenPrice(contractID string) (price, vol24 decimal.Decimal) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.dexscreener.com/latest/dex/token/%s", contractID)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("请求 token %s 价格失败: %s", contractID, err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("读取 token %s 响应失败: %s", contractID, err)
		return
	}
	var r dexscreenerResp
	err = json.Unmarshal(body, &r)
	if err != nil {
		log.Errorf("解析 token %s 价格失败: %s, body: %s",
			contractID, err, string(body[:min(len(body), 200)]))
		return
	}
	for _, pair := range r.Pairs {
		p, _ := decimal.NewFromString(pair.PriceUsd)
		v := decimal.NewFromFloat(pair.Volume.H24)
		if p.GreaterThan(price) {
			price = p
			vol24 = v
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
