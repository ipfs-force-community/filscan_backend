package price_syncer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gozelle/async/forever"
	logging "github.com/gozelle/logger"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
)

type CMCResponse struct {
	Data struct {
		Filecoin struct {
			Quote struct {
				USD struct {
					Price            float64   `json:"price"`
					PercentChange24h float64   `json:"percent_change_24h"`
					Time             time.Time `json:"last_updated"`
				} `json:"USD"`
			} `json:"quote"`
		} `json:"2280"`
	} `json:"data"`
}

type FilpriceTask struct {
	log  *logging.Logger
	repo repository.FilPriceRepo
}

func (a *FilpriceTask) Run() {
	a.log.Info("filprice task start to run")
	forever.Run(10*time.Minute, func() {
		err := a.run()
		if err != nil {
			a.log.Errorf("miner location task exec error: %s", err)
		}
	})
}

func (a *FilpriceTask) run() error {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", "https://pro-api.coinmarketcap.com/v2/cryptocurrency/quotes/latest?id=2280", nil)
	token := "6510f4f2-e831-4526-8be1-d2c2d0557793"

	if s := os.Getenv("CMC_TOKEN"); s != "" {
		token = s
	}
	req.Header.Set("X-CMC_PRO_API_KEY", token)
	resp, err := client.Do(req)
	if err != nil {
		a.log.Errorf("get fil price failed: %w", err)
		return err
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	res := CMCResponse{}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		return err
	}
	usd := res.Data.Filecoin.Quote.USD
	err = a.repo.SaveFilPrice(context.TODO(), usd.Price, usd.PercentChange24h, usd.Time)
	if err != nil {
		a.log.Errorf("save fil price failed: %w", err)
		return err
	}
	return nil
}

func NewFilpriceTask(repo repository.FilPriceRepo) *FilpriceTask {
	return &FilpriceTask{
		repo: repo,
		log:  logging.NewLogger("fullactors"),
	}
}
