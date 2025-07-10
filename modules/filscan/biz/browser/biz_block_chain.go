package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"sync"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gorm.io/gorm"
)

var once sync.Once

func initGlobalTagMap(adapter londobell.Adapter, db *gorm.DB) error {
	convertor.GlobalTagMap = map[string]string{}
	dal := dal.NewAddrTagDal(db)
	addrs, err := dal.GetAllAddrTags(context.TODO())
	if err != nil {
		return err
	}

	for i := range addrs {
		convertor.GlobalTagMap[addrs[i].Address] = addrs[i].Tag

		actor, err := adapter.Actor(context.TODO(), chain.SmartAddress(addrs[i].Address), nil)
		if err != nil {
			return err
		}
		convertor.GlobalTagMap[actor.ActorID] = addrs[i].Tag
		convertor.GlobalTagMap[actor.ActorAddr] = addrs[i].Tag
	}

	return nil
}

func NewBlockChainBiz(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB, conf *config.Config) *BlockChainBiz {
	once.Do(func() {
		err := initGlobalTagMap(adapter, db)
		if err != nil {
			panic(fmt.Errorf("init global addr tag map failed: %w", err))
		}

		log.Infof("init global map success , %#v", convertor.GlobalTagMap)
	})

	return &BlockChainBiz{
		AccountBiz:         NewAccountBiz(agg, adapter, db, conf),
		BlockChainAclImpl:  acl.NewBlockChainAclImpl(agg, adapter, dal.NewNFTQueryer(db), conf),
		RichAccountRankBiz: NewRichAccountRankBiz(adapter, db),
		DealDetailsBiz:     NewDealDetailsBiz(agg, db),
		Redis:              redis.NewRedis(conf),
		FilPriceRepo:       dal.NewFilPriceDal(db),
	}
}

var _ filscan.BlockChainAPI = (*BlockChainBiz)(nil)

type BlockChainBiz struct {
	*acl.BlockChainAclImpl
	*AccountBiz
	*RichAccountRankBiz
	*DealDetailsBiz
	*redis.Redis
	repository.FilPriceRepo
}

func (b BlockChainBiz) GasSummary(ctx context.Context, req struct{}) (resp *filscan.GasSummaryResponse, err error) {
	return &filscan.GasSummaryResponse{
		Sum:         decimal.NewFromInt(rand.Int63n(math.MaxInt64)),
		ContractGas: decimal.NewFromInt(rand.Int63n(math.MaxInt64)),
		Others:      decimal.NewFromInt(rand.Int63n(math.MaxInt64)),
	}, nil
}

func (b BlockChainBiz) FilPrice(ctx context.Context, req struct{}) (resp *filscan.FilPriceResponse, err error) {
	fil, err := b.FilPriceRepo.LatestPrice(ctx)
	if err != nil {
		return nil, err
	}

	return &filscan.FilPriceResponse{
		Price:            fil.Price,
		PercentChange24h: fil.PercentChange,
	}, nil
}

func (b BlockChainBiz) FinalHeight(ctx context.Context, req filscan.FinalHeightRequest) (resp filscan.FinalHeightResponse, err error) {
	tipset, err := b.GetLatestTipset(ctx)
	if err != nil {
		return
	}

	if tipset != nil {
		resp.Height = tipset[0].ID
		resp.BlockTime = chain.Epoch(tipset[0].ID).Unix()
		resp.BaseFee = tipset[0].BaseFee
	}
	return
}

func (b BlockChainBiz) TipsetStateTree(ctx context.Context, req filscan.TipsetStateTreeRequest) (resp filscan.TipsetStateTreeResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	resp, err = b.GetTipsetStateTree(ctx, req.Filters)
	if err != nil {
		return
	}
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) TipsetDetail(ctx context.Context, req filscan.TipsetDetailRequest) (resp filscan.TipsetDetailResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	tipsetDetail, err := b.GetTipsetDetail(ctx, req.Height)
	if err != nil {
		return
	}
	resp.TipsetDetail = tipsetDetail
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) LatestBlocks(ctx context.Context, req filscan.LatestBlocksRequest) (resp filscan.LatestBlocksResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	list, err := b.GetLatestBlockList(ctx, req.Filters)
	if err != nil {
		return
	}
	resp = list
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) BlockDetails(ctx context.Context, req filscan.BlockDetailsRequest) (resp filscan.BlockDetailsResponse, err error) {
	blockDetails, err := b.GetBlockDetails(ctx, req.BlockCid)
	if err != nil {
		return
	}
	if blockDetails != nil {
		resp.BlockDetails = blockDetails
	}
	return
}

func (b BlockChainBiz) MessagesByBlock(ctx context.Context, req filscan.MessagesByBlockRequest) (resp filscan.MessagesByBlockResponse, err error) {
	blockMessages, err := b.GetMessagesByBlock(ctx, req.BlockCid, req.Filters)
	if err != nil {
		return
	}
	if blockMessages != nil {
		resp = *blockMessages
	}
	return
}

func (b BlockChainBiz) LatestMessages(ctx context.Context, req filscan.LatestMessagesRequest) (resp filscan.LatestMessagesResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	messageList, err := b.GetLatestMessageList(ctx, req.Filters)
	if err != nil {
		return
	}
	if messageList != nil {
		resp = *messageList
	}
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) MessageDetails(ctx context.Context, req filscan.MessageDetailsRequest) (resp filscan.MessageDetailsResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	var messageCid string
	if regexp.MustCompile("^0x[0-9a-fA-F]{64}$").MatchString(req.MessageCid) {
		var messagePool *filscan.MessagesPoolResponse
		messagePool, err = b.GetMessagePool(ctx, req.MessageCid, nil)
		if err != nil {
			return
		}
		if messagePool != nil && messagePool.MessagesPoolList != nil {
			messageCid = messagePool.MessagesPoolList[0].MessageBasic.Cid
		} else {
			messageCid, err = b.GetMessageCidByHash(ctx, req.MessageCid)
			if err != nil {
				return
			}
		}
	} else {
		messageCid = req.MessageCid
	}

	messageDetails, err := b.GetMessageDetails(ctx, messageCid)
	if err != nil {
		return
	}
	if messageDetails != nil {
		if messageDetails.MessageBasic.Cid != messageCid {
			messageDetails.Replaced = true
		}
		resp.MessageDetails = messageDetails
	}

	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) LargeTransfers(ctx context.Context, req filscan.LargeTransfersRequest) (resp filscan.LargeTransfersResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	largeAmountList, err := b.GetTransferLargeAmount(ctx, req.Filters)
	if err != nil {
		return
	}
	if largeAmountList != nil {
		resp = *largeAmountList
	}
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) SearchMarketDeals(ctx context.Context, req filscan.SearchMarketDealsRequest) (resp filscan.SearchMarketDealsResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	var dealsList *filscan.SearchMarketDealsResponse
	if req.Input == "" {
		dealsList, err = b.GetDealsList(ctx, req.Filters)
		if err != nil {
			return
		}
		if dealsList != nil {
			resp = *dealsList
		}
	} else {
		matchAccountID := chain.SmartAddress(req.Input).IsValid()
		matchDealID := regexp.MustCompile("^[1-9]\\d*$")
		if matchAccountID {
			dealsList, err = b.GetDealsListByAddr(ctx, chain.SmartAddress(req.Input), req.Filters)
			if err != nil {
				return
			}
			if dealsList != nil {
				resp = *dealsList
			}
		}
		if matchDealID.MatchString(req.Input) {
			var inputNum int64
			inputNum, err = strconv.ParseInt(req.Input, 10, 64)
			if err != nil {
				return
			}
			var marketDeal *filscan.MarketDeal
			marketDeal, err = b.GetDealByID(ctx, inputNum)
			if err != nil {
				return
			}
			if marketDeal != nil {
				resp.MarketDealsList = append(resp.MarketDealsList, marketDeal)
				resp.TotalCount = int64(len(resp.MarketDealsList))
			}
		}
	}

	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) MessagesPool(ctx context.Context, req filscan.MessagesPoolRequest) (resp filscan.MessagesPoolResponse, err error) {

	messagePool, err := b.GetMessagePool(ctx, req.Cid, &req.Filters)
	if err != nil {
		return
	}
	if messagePool != nil {
		resp = *messagePool
	}
	return
}

func (b BlockChainBiz) AllMethods(ctx context.Context, req filscan.AllMethodRequest) (resp filscan.AllMethodResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	methodNameList, err := b.GetAllMethodName(ctx)
	if err != nil {
		return
	}
	if methodNameList != nil {
		resp.MethodNameList = methodNameList
	}
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (b BlockChainBiz) AllMethodsByBlock(ctx context.Context, req filscan.AllMethodsByBlockRequest) (resp filscan.AllMethodsByBlockResponse, err error) {
	methodNameList, err := b.GetAllMethodsForBlockMessage(ctx, req.Cid)
	if err != nil {
		return
	}
	if methodNameList != nil {
		resp.MethodNameList = methodNameList
	}
	return
}

func (b BlockChainBiz) AllMethodsByMessagePool(ctx context.Context, req filscan.AllMethodsByMessagePoolRequest) (resp filscan.AllMethodsByMessagePoolResponse, err error) {
	cacheKey, err := b.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := b.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	methodNameList, err := b.GetAllMethodsForMessagePool(ctx)
	if err != nil {
		return
	}
	if methodNameList != nil {
		resp.MethodNameList = methodNameList
	}
	err = b.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}
