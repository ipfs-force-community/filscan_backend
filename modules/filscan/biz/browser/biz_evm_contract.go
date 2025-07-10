package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"

	"gorm.io/gorm"
)

const EVMStatisticCacheKey = "EVMStatisticCacheKey"
const EVMTxsCacheKey = "EVMTxsCacheKey"
const EVMGasCacheKey = "EVMGasCacheKey"

func NewContract(db *gorm.DB, agg londobell.Agg, adapter londobell.Adapter, conf *config.Config) *ContractBiz {
	res := ContractBiz{
		agg:          agg,
		adapter:      adapter,
		redisCache:   redis.NewRedis(conf),
		evmTransfer:  dal.NewEVMTransferDal(db),
		evmEvent:     dal.NewEvmSignatureDal(db),
		gasTrendRepo: dal.NewGas24hTrendBizDal(db),
	}
	go func() {
		ticker := time.NewTimer(time.Minute * 30)
		for {
			select {
			case <-ticker.C:
			}
			ticker.Reset(time.Minute * 30)

			ctx := context.TODO()

			epoch := chain.CurrentEpoch()
			totalCount, err := res.evmTransfer.CountUniqueContracts(ctx, epoch)
			if err != nil {
				log.Error(err)
				continue
			}
			totalCountBefore, err := res.evmTransfer.CountUniqueContracts(ctx, epoch-2880)
			if err != nil {
				log.Error(err)
				continue
			}
			totalTxs, err := res.evmTransfer.CountTxsOfContracts(ctx, epoch)
			if err != nil {
				log.Error(err)
				continue
			}
			totalTxsBefore, err := res.evmTransfer.CountTxsOfContracts(ctx, epoch-2880)
			if err != nil {
				log.Error(err)
				continue
			}

			totalUsers, err := res.evmTransfer.CountUniqueUsers(ctx, epoch)
			if err != nil {
				log.Error(err)
				continue
			}

			totalUsersBefore, err := res.evmTransfer.CountUniqueUsers(ctx, epoch-2880)
			if err != nil {
				log.Error(err)
				continue
			}

			unique, err := res.evmTransfer.CountVerifiedContracts(ctx)
			if err != nil {
				log.Error(err)
				continue
			}

			re := filscan.EvmContractSummaryResponse{
				TotalContract:            totalCount,
				TotalContractChangeIn24h: totalCount - totalCountBefore,
				ContractTxs:              totalTxs,
				ContractTxsChangeIn24h:   totalTxs - totalTxsBefore,
				ContractUsers:            totalUsers,
				ContractUsersChangeIn24h: totalUsers - totalUsersBefore,
				VerifiedContracts:        unique,
			}

			res.redisCache.Set(EVMStatisticCacheKey, re, time.Minute*60) //nolint
		}
	}()
	return &res
}

var _ filscan.EvmContractAPI = (*ContractBiz)(nil)

type ContractBiz struct {
	agg          londobell.Agg
	adapter      londobell.Adapter
	redisCache   *redis.Redis
	evmTransfer  repository.EvmTransferRepo
	evmEvent     repository.EvmSignatureRepo
	gasTrendRepo repository.Gas24hTrendBizRepo
}

func (c ContractBiz) EvmContractSummary(ctx context.Context, req struct{}) (resp filscan.EvmContractSummaryResponse, err error) {
	if a, b := c.redisCache.GetCacheResult(EVMStatisticCacheKey); b == nil && a != nil {
		err = json.Unmarshal(a, &resp)
		if err == nil {
			return resp, nil
		}
	}
	epoch := chain.CurrentEpoch()
	totalCount, err := c.evmTransfer.CountUniqueContracts(ctx, epoch)
	if err != nil {
		return
	}
	totalCountBefore, err := c.evmTransfer.CountUniqueContracts(ctx, epoch-2880)
	if err != nil {
		return
	}
	totalTxs, err := c.evmTransfer.CountTxsOfContracts(ctx, epoch)
	if err != nil {
		return
	}
	totalTxsBefore, err := c.evmTransfer.CountTxsOfContracts(ctx, epoch-2880)
	if err != nil {
		return
	}

	totalUsers, err := c.evmTransfer.CountUniqueUsers(ctx, epoch)
	if err != nil {
		return
	}

	totalUsersBefore, err := c.evmTransfer.CountUniqueUsers(ctx, epoch-2880)
	if err != nil {
		return
	}

	unique, err := c.evmTransfer.CountVerifiedContracts(ctx)
	if err != nil {
		return
	}

	re := filscan.EvmContractSummaryResponse{
		TotalContract:            totalCount,
		TotalContractChangeIn24h: totalCount - totalCountBefore,
		ContractTxs:              totalTxs,
		ContractTxsChangeIn24h:   totalTxs - totalTxsBefore,
		ContractUsers:            totalUsers,
		ContractUsersChangeIn24h: totalUsers - totalUsersBefore,
		VerifiedContracts:        unique,
	}

	c.redisCache.Set(EVMStatisticCacheKey, re, time.Minute*30) //nolint
	return re, nil
}

func (c ContractBiz) EvmGasTrend(ctx context.Context, req filscan.EvmGasTrendReq) (resp filscan.EvmGasTrendRes, err error) {
	if a, b := c.redisCache.GetCacheResult(fmt.Sprintf("%s-%s", EVMGasCacheKey, req.Interval)); b == nil && a != nil {
		err = json.Unmarshal(a, &resp)
		if err == nil {
			return resp, nil
		}
	}
	epoch, err := c.gasTrendRepo.GetLatestMethodGasCostEpoch(ctx)
	if err != nil {
		log.Errorf("get latest epoch failed: %s", err.Error())
		epoch = chain.Epoch(0)
	}
	var list []filscan.EvmGas
	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, epoch)
	if err != nil {
		return
	}
	epochs := it.Points()
	//fmt.Println(epochs)

	if len(epochs) < 2 {
		return filscan.EvmGasTrendRes{}, fmt.Errorf("valid points less than 2")
	}

	for i := 1; i < len(epochs); i++ {
		entities, err := c.gasTrendRepo.GetMethodGasFees(ctx, chain.NewLCRORange(epochs[i-1], epochs[i]))
		if err != nil {
			log.Errorf("get method gas fees failed: %s", err.Error())
			return resp, err
		}
		for j := range entities {
			if entities[j].Method == "InvokeContract" {
				list = append(list, filscan.EvmGas{
					Timestamp: epochs[i].Unix(),
					TxsGas:    entities[j].GasCost,
				})
				break
			}
		}
	}

	resp = filscan.EvmGasTrendRes{
		Epoch:     epoch.Int64(),
		BlockTime: epoch.Time().Unix(),
	}
	resp.EvmGasTrend = list
	c.redisCache.Set(fmt.Sprintf("%s-%s", EVMGasCacheKey, req.Interval), resp, time.Hour) //nolint
	return resp, nil
}

func (c ContractBiz) EvmTxsHistory(ctx context.Context, req filscan.EvmTxsHistoryReq) (resp filscan.EvmTxsHistoryRes, err error) {
	if a, b := c.redisCache.GetCacheResult(fmt.Sprintf("%s-%s", EVMTxsCacheKey, req.Interval)); b == nil && a != nil {
		err = json.Unmarshal(a, &resp)
		if err == nil {
			return resp, nil
		}
	}
	epoch := chain.CurrentEpoch()
	list := []filscan.EvmTxs{}
	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, epoch)
	if err != nil {
		return
	}
	epochs := it.Points()
	//fmt.Println(epochs)
	if len(epochs) < 2 {
		return filscan.EvmTxsHistoryRes{}, fmt.Errorf("valid points less than 2")
	}

	trans, err := c.evmTransfer.GetTxsOfContractsByRange(ctx, epochs[0], epochs[len(epochs)-1])
	if err != nil {
		return resp, err
	}
	sort.Slice(trans, func(i, j int) bool {
		return trans[i].Epoch < trans[j].Epoch
	})
	index := 0
	cnt := int64(0)
	transLen := len(trans)
	for i := 1; i < len(epochs); i++ {
		// 判断每个epoch时候，统计数量
		for index < transLen && trans[index].Epoch < epochs[i].Int64() {
			index++
			cnt++
		}
		list = append(list, filscan.EvmTxs{
			Timestamp: epochs[i].Unix(),
			TxsCount:  cnt,
		})
		cnt = 0
	}

	re := filscan.EvmTxsHistoryRes{EvmTxsHistory: list}
	c.redisCache.Set(fmt.Sprintf("%s-%s", EVMTxsCacheKey, req.Interval), re, time.Hour) //nolint
	return re, nil
}

func (c ContractBiz) EvmContractList(ctx context.Context, request filscan.EvmContractRequest) (resp filscan.EvmContractResponse, err error) {
	transferList, total, err := c.evmTransfer.GetEvmTransferStatsList(ctx, request.Page, request.Limit, request.Field, request.Sort, "")
	if err != nil {
		return
	}
	if transferList != nil {
		for index, transfer := range transferList {
			var contract *filscan.EvmContract
			contract = assembler.EvmContract{}.EvmTransferStatToContract(transfer, request.Page, request.Limit, index)
			resp.EvmContractList = append(resp.EvmContractList, contract)
		}
		resp.Total = int64(total)
		resp.UpdateTime = chain.CurrentEpoch().CurrentHour().Unix()
	}

	return
}

func (c ContractBiz) ActorEventsList(ctx context.Context, request filscan.ActorEventsListRequest) (resp filscan.ActorEventsListResponse, err error) {
	events, err := c.agg.EventsForActor(ctx, chain.SmartAddress(request.ActorID), request.Page, request.Limit)
	if err != nil {
		return
	}

	if events == nil || events.EventsForActor == nil {
		return
	}
	var hexSignature []string
	var eventList []*filscan.Event
	for _, event := range events.EventsForActor {
		hexSignature = append(hexSignature, event.Topics[0])
		eventList = append(eventList, assembler.EvmContract{}.EvmSignatureToEvent(event))
	}
	var signatures []*po.EvmEventSignature
	if hexSignature != nil {
		signatures, err = c.evmEvent.GetEvmEventSignatures(ctx, hexSignature)
		if err != nil {
			return
		}
	}

	if signatures == nil || eventList == nil {
		return
	}
	for _, event := range eventList {
		for _, signature := range signatures {
			if event.Topics[0] == signature.HexSignature {
				event.EventName = signature.TextSignature
			}
		}
	}
	resp.EventList = eventList
	resp.TotalCount = int64(events.TotalCount)
	return
}
