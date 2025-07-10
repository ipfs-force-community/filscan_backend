package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
)

const ContractUsersTrendCacheKey = "ContractUsersTrendCacheKey"
const ContractCntsTrendCacheKey = "ContractCntsTrendCacheKey"
const ContractBalanceTrendCacheKey = "ContractBalanceTrendCacheKey"

var _ filscan.StatisticContractTrend = (*ContractTrendBiz)(nil)

func NewContractTrendBiz(se repository.SyncerGetter, repo repository.ContractTrendBizRepo, conf *config.Config) *ContractTrendBiz {
	res := &ContractTrendBiz{
		se:         se,
		repo:       repo,
		redisCache: redis.NewRedis(conf),
	}
	go func() {
		ticker := time.NewTimer(time.Minute * 30)
		dur := []string{"24h", "7d", "1m"}
		for {
			select {
			case <-ticker.C:
			}
			ticker.Reset(time.Minute * 30)

			ctx := context.TODO()

			epoch, err := res.se.GetSyncer(ctx, syncer.EvmContractSyncer)
			if err != nil {
				log.Error(err)
			}
			for _, t := range dur {
				var it interval.Interval
				it, err = interval.ResolveInterval(t, chain.Epoch(epoch.Epoch))
				if err != nil {
					return
				}

				r, err := res.repo.GetContractUsersByEpochs(ctx, it.Points())
				if err != nil {
					return
				}

				resp := &filscan.ContractUsersTrendResponse{
					Epoch:     epoch.Epoch,
					BlockTime: chain.Epoch(epoch.Epoch).Time().Unix(),
				}

				for _, v := range r {
					resp.Items = append(resp.Items, &filscan.ContractUsersTrend{
						ContractUsers: v.ContractUsers,
						BlockTime:     v.BlockTime,
					})
				}
				sort.Slice(resp.Items, func(i, j int) bool {
					return resp.Items[i].BlockTime < resp.Items[j].BlockTime
				})
				res.redisCache.Set(fmt.Sprintf("%s-%s", ContractUsersTrendCacheKey, t), resp, time.Hour) //nolint
				time.Sleep(2 * time.Minute)                                                              //不让同时更新太多

				epoches := interval.ToHourlyPoint(it.Points())
				r3, err := res.repo.GetContractBalanceByEpochs(ctx, epoches)
				if err != nil {
					return
				}

				resp3 := &filscan.ContractBalanceTrendResponse{
					Epoch:     epoch.Epoch,
					BlockTime: chain.Epoch(epoch.Epoch).Time().Unix(),
				}

				for _, v := range r3 {
					resp3.Items = append(resp3.Items, &filscan.ContractBalanceTrend{
						ContractTotalBalance: v.ContractTotalBalance,
						BlockTime:            v.BlockTime,
					})
				}
				sort.Slice(resp.Items, func(i, j int) bool {
					return resp.Items[i].BlockTime < resp.Items[j].BlockTime
				})
				res.redisCache.Set(fmt.Sprintf("%s-%s", ContractBalanceTrendCacheKey, t), resp3, time.Hour) //nolint
			}
		}
	}()
	return res
}

type ContractTrendBiz struct {
	se         repository.SyncerGetter
	repo       repository.ContractTrendBizRepo
	redisCache *redis.Redis
}

func (c ContractTrendBiz) ContractCntTrend(ctx context.Context, req filscan.ContractCntTrendRequest) (resp *filscan.ContractCntTrendResponse, err error) {
	if a, b := c.redisCache.GetCacheResult(fmt.Sprintf("%s-%s", ContractCntsTrendCacheKey, req.Interval)); b == nil && a != nil {
		err = json.Unmarshal(a, &resp)
		if err == nil {
			return resp, nil
		}
	}
	current, err := c.se.GetSyncer(ctx, syncer.EvmContractSyncer)
	if err != nil || current == nil {
		return
	}

	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, chain.Epoch(current.Epoch))
	if err != nil {
		return
	}

	epoches := interval.ToHourlyPoint(it.Points())
	contractCntList, err := c.repo.GetContractCntByEpochs(ctx, epoches)
	if err != nil {
		return
	}
	// 用于存储中间的结果（这一步是为了让当发生数据缺失的时候来用0来补充增量数据）
	tmpContractCntList := make([]*bo.ContractCnt, len(contractCntList)-1)
	finalContractCntList := make([]*bo.ContractCnt, len(epoches)-1)
	res := contractCntList[0].Cnts
	for i := 1; i < len(contractCntList); i++ {
		pre := res
		res = contractCntList[i].Cnts
		tmpContractCntList[i-1] = &bo.ContractCnt{
			Epoch: contractCntList[i].Epoch,
			Cnts:  res - pre,
		}
	}
	if len(contractCntList) != len(epoches) {
		index := 0
		for i := 1; i < len(epoches) && index < len(tmpContractCntList); i++ {
			if epoches[i] == tmpContractCntList[index].Epoch {
				finalContractCntList[i-1] = tmpContractCntList[index]
				index++
			} else {
				finalContractCntList[i-1] = &bo.ContractCnt{
					Epoch: epoches[i],
					Cnts:  0,
				}
			}
		}
	} else {
		finalContractCntList = tmpContractCntList
	}

	resp = &filscan.ContractCntTrendResponse{
		Epoch:     current.Epoch,
		BlockTime: chain.Epoch(current.Epoch).Time().Unix(),
	}

	for _, v := range finalContractCntList {
		if v != nil {
			resp.Items = append(resp.Items, &filscan.ContractCntTrend{
				ContractCnts: v.Cnts,
				BlockTime:    v.Epoch.Unix(),
			})
		}
	}
	c.redisCache.Set(fmt.Sprintf("%s-%s", ContractCntsTrendCacheKey, req.Interval), resp, time.Hour) //nolint
	return
}

func (c ContractTrendBiz) ContractUsersTrend(ctx context.Context, req filscan.ContractUsersTrendRequest) (resp *filscan.ContractUsersTrendResponse, err error) {
	if a, b := c.redisCache.GetCacheResult(fmt.Sprintf("%s-%s", ContractUsersTrendCacheKey, req.Interval)); b == nil && a != nil {
		err = json.Unmarshal(a, &resp)
		if err == nil {
			return resp, nil
		}
	}
	current, err := c.se.GetSyncer(ctx, syncer.EvmContractSyncer)
	if err != nil {
		return
	}
	if current == nil {
		return
	}

	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, chain.Epoch(current.Epoch))
	if err != nil {
		return
	}

	r, err := c.repo.GetContractUsersByEpochs(ctx, it.Points())
	if err != nil {
		return
	}

	resp = &filscan.ContractUsersTrendResponse{
		Epoch:     current.Epoch,
		BlockTime: chain.Epoch(current.Epoch).Time().Unix(),
	}

	for _, v := range r {
		resp.Items = append(resp.Items, &filscan.ContractUsersTrend{
			ContractUsers: v.ContractUsers,
			BlockTime:     v.BlockTime,
		})
	}

	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].BlockTime < resp.Items[j].BlockTime
	})
	c.redisCache.Set(fmt.Sprintf("%s-%s", ContractUsersTrendCacheKey, req.Interval), resp, time.Hour) //nolint
	return
}

func (c ContractTrendBiz) ContractTxsTrend(ctx context.Context, req filscan.ContractTxsTrendRequest) (resp *filscan.ContractTxsTrendResponse, err error) {
	current, err := c.se.GetSyncer(ctx, syncer.EvmContractSyncer)
	if err != nil || current == nil {
		return
	}

	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, chain.Epoch(current.Epoch))
	if err != nil {
		return
	}

	r, err := c.repo.GetContractTxsByEpochs(ctx, it.Points())
	if err != nil {
		return
	}

	resp = &filscan.ContractTxsTrendResponse{
		Epoch:     current.Epoch,
		BlockTime: chain.Epoch(current.Epoch).Time().Unix(),
	}

	for _, v := range r {
		resp.Items = append(resp.Items, &filscan.ContractTxsTrend{
			ContractTxs: v.ContractTxs,
			BlockTime:   v.BlockTime,
		})
	}
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].BlockTime < resp.Items[j].BlockTime
	})

	return
}

func (c ContractTrendBiz) ContractBalanceTrend(ctx context.Context, req filscan.ContractBalanceTrendRequest) (resp *filscan.ContractBalanceTrendResponse, err error) {
	if a, b := c.redisCache.GetCacheResult(fmt.Sprintf("%s-%s", ContractBalanceTrendCacheKey, req.Interval)); b == nil && a != nil {
		err = json.Unmarshal(a, &resp)
		if err == nil {
			return resp, nil
		}
	}
	current, err := c.se.GetSyncer(ctx, syncer.EvmContractSyncer)
	if err != nil || current == nil {
		return
	}

	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, chain.Epoch(current.Epoch))
	if err != nil {
		return
	}

	epoches := interval.ToHourlyPoint(it.Points())
	r, err := c.repo.GetContractBalanceByEpochs(ctx, epoches)
	if err != nil {
		return
	}

	resp = &filscan.ContractBalanceTrendResponse{
		Epoch:     current.Epoch,
		BlockTime: chain.Epoch(current.Epoch).Time().Unix(),
	}

	for _, v := range r {
		resp.Items = append(resp.Items, &filscan.ContractBalanceTrend{
			ContractTotalBalance: v.ContractTotalBalance,
			BlockTime:            v.BlockTime,
		})
	}
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].BlockTime < resp.Items[j].BlockTime
	})
	c.redisCache.Set(fmt.Sprintf("%s-%s", ContractBalanceTrendCacheKey, req.Interval), resp, time.Hour) //nolint
	return
}
