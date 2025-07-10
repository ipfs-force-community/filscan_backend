package impl

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	"net/http"
	"net/url"
	
	"github.com/gozelle/fastjson"
	"github.com/gozelle/logger"
	"github.com/gozelle/resty"
	"github.com/gozelle/resty/agent"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewMinerAggImpl(host string) (a *MinerAggImpl, err error) {
	u, err := url.Parse(host)
	if err != nil {
		return
	}
	a = &MinerAggImpl{
		Agent: agent.NewAgent(resty.New(), u),
	}
	a.Agent.SetLogger(logger.WithSkip(logger.NewLogger("miner-agg"), 1))
	return
}

var _ londobell.MinerAgg = (*MinerAggImpl)(nil)

type MinerAggImpl struct {
	*agent.Agent
}

func (m MinerAggImpl) responseFilter(resp *resty.Response) (data []byte, err error) {
	
	if resp.IsError() {
		err = fmt.Errorf("%s", resp.Status())
		return
	}
	
	r, err := fastjson.ParseBytes(resp.Body())
	if err != nil {
		return
	}
	
	code := r.GetInt("code")
	if err != nil {
		return
	}
	if code != 0 {
		err = fmt.Errorf("code: %d is not 0, msg: <%s>", code, string(r.GetStringBytes("msg")))
		return
	}
	
	data = []byte(r.Get("data").String())
	return
}

func (m MinerAggImpl) PeriodBlockRewards(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *londobell.PeriodBlockRewardsResp, err error) {
	err = m.Debug().Request(ctx, http.MethodPost, "/aggregators/miner/periodblockrewards",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodWinCounts(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *londobell.PeriodWinCountsResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/periodwincounts",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodGasCost(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r []*londobell.PeriodGasCostResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/periodgascost",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodGasCostForPublishDeals(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r []*londobell.PeriodGasCostResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/periodgascostforpublishdeals",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodPunishments(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *londobell.PeriodPunishmentsResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/periodpunishments",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodSectorDiff(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *londobell.PeriodSectorDiffResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/periodsectorsdiff",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodPledgeDiff(ctx context.Context, miner chain.SmartAddress, epochs chain.LCRORange) (r *londobell.PeriodPledgeDiffResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/periodpledgediff",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodExpirations(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r []*londobell.Expiration, err error) {
	err = m.Debug().Request(ctx, http.MethodPost, "/aggregators/miner/periodexpirations",
		agent.WithRequestBody(map[string]interface{}{
			"addr": miner.CrudeAddress(),
			"end":  epoch.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) QaPowerHistory(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r *londobell.QaPowerHistoryResp, err error) {
	
	var items []*londobell.QaPowerHistoryResp
	
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/qapowerhistory",
		agent.WithRequestBody(map[string]interface{}{
			"addr": miner.CrudeAddress(),
			"end":  epoch.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&items)
	if err != nil {
		return
	}
	if len(items) > 0 {
		r = items[0]
	}
	return
}

func (m MinerAggImpl) SectorHealthHistory(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r *londobell.SectorHealthHistoryResp, err error) {
	err = m.Request(ctx, http.MethodPost, "/aggregators/miner/sectorhealthhistory",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  miner.CrudeAddress(),
			"start": epoch.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&r)
	if err != nil {
		return
	}
	return
}

func (m MinerAggImpl) PeriodBill(ctx context.Context, addr chain.SmartAddress, epochs chain.LCRORange) (r *londobell.PeriodBillResp, err error) {
	type Record struct {
		Income  string
		Pay     string
		GasCost string
	}
	var record Record
	err = m.Request(ctx, http.MethodPost, "/aggregators/account/periodbill",
		agent.WithRequestBody(map[string]interface{}{
			"addr":  addr.CrudeAddress(),
			"start": epochs.GteBegin.Int64(),
			"end":   epochs.LtEnd.Int64(),
		}),
		agent.WithResponseFilter(m.responseFilter),
	).Bind(&record)
	if err != nil {
		return
	}
	
	r = &londobell.PeriodBillResp{}
	if record.Income != "" {
		r.Income, err = decimal.NewFromString(record.Income)
		if err != nil {
			return
		}
	}
	if record.Pay != "" {
		r.Pay, err = decimal.NewFromString(record.Pay)
		if err != nil {
			return
		}
	}
	if record.GasCost != "" {
		r.GasCost, err = decimal.NewFromString(record.GasCost)
		if err != nil {
			return
		}
	}
	
	return
}
