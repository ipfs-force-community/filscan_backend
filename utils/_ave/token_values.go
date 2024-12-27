package _ave

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/gozelle/logger"
	"io"
	"net/http"
	"sync"
	"time"

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

func RefreshToken(contractID string, td time.Duration) {
	time.Sleep(td)
	go RefreshToken(contractID, td)
	client := &http.Client{}
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://openapi.opencc.xyz/api/v1/tokens/%s-filecoin", contractID), nil)
	req.Header.Set("Ave-Auth", "0x1qqgdcaf564e4bfda1c483642db72007871324gdy")
	resp, err := client.Do(req)
	if err != nil {
		log.Error(err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return
	}
	A := struct {
		Code int `json:"code"`
		Data struct {
			LastPrice string `json:"price"`
			Volume    string `json:"turnover_24h"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(body, &A)
	if err != nil {
		log.Error(err)
		return
	}

	vol24, _ := decimal.NewFromString(A.Data.Volume)
	price, _ := decimal.NewFromString(A.Data.LastPrice)
	tc.Set(contractID, ExchangeInfo{
		LatestPrice: price,
		Vol24:       vol24,
	}, 30*time.Minute)
}

func GetTokenExchangeInfo(contractID string) ExchangeInfo {
	v, _, _ := singleFlight.Do(contractID, func() (interface{}, error) {
		res := tc.Get(contractID)
		if res == nil {
			res = &ExchangeInfo{
				LatestPrice: decimal.Zero,
			}
			for i := 0; i < 5; i++ {
				client := &http.Client{}
				req, _ := http.NewRequest("GET", fmt.Sprintf("https://openapi.opencc.xyz/api/v1/tokens/%s-filecoin", contractID), nil)
				req.Header.Set("Ave-Auth", "0x1qqgdcaf564e4bfda1c483642db72007871324gdy")
				http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
				resp, err := client.Do(req)
				if err != nil {
					log.Error(err)
					time.Sleep(2 * time.Second)
					continue
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Error(err)
					break
				}
				A := struct {
					Code int `json:"code"`
					Data struct {
						LastPrice string `json:"price"`
						Volume    string `json:"turnover_24h"`
					} `json:"data"`
				}{}

				err = json.Unmarshal(body, &A)
				if err != nil {
					log.Error(err)
					break
				}

				vol24, _ := decimal.NewFromString(A.Data.Volume)
				price, _ := decimal.NewFromString(A.Data.LastPrice)
				tc.Set(contractID, ExchangeInfo{
					LatestPrice: price,
					Vol24:       vol24,
				}, 30*time.Minute)
				res.LatestPrice = price
				res.Vol24 = vol24
				go RefreshToken(contractID, time.Minute*30)
				break
			}
		}
		return *res, nil
	})
	return v.(ExchangeInfo)
}
