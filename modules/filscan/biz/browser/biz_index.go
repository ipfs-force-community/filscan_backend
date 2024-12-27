package browser

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns/providers"

	"github.com/gozelle/async/forever"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

func NewIndexBiz(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB, conf *config.Config) *IndexBiz {
	b := &IndexBiz{
		baseFeeTrendDal: dal.NewGasPerTDal(db),
		messageCountDal: dal.NewMessageCountTaskDal(db),
		IndexAclImpl:    acl.NewIndexAclImpl(agg, adapter),
		BlockChainBiz:   NewBlockChainBiz(agg, adapter, db, conf),
		RankBiz:         NewRankBiz(db, conf),
		StatisticBiz:    NewStatisticBiz(db, adapter, conf),
		gasTrendRepo:    dal.NewGas24hTrendBizDal(db),
		bannerRepo:      dal.NewBannerIndicatorDal(db),
	}
	go b.cacheTotalIndicators()
	return b
}

var _ filscan.IndexAPI = (*IndexBiz)(nil)

type IndexBiz struct {
	*acl.IndexAclImpl
	*BlockChainBiz
	*RankBiz
	*StatisticBiz
	baseFeeTrendDal repository.GasPerTRepo
	messageCountDal repository.MessageCountTaskRepo
	totalIndicators *filscan.TotalIndicatorsResponse
	lastCacheTime   time.Time
	once            sync.Once
	gasTrendRepo    repository.Gas24hTrendBizRepo
	bannerRepo      repository.BannerIndicatorRepo
}

func (i *IndexBiz) cacheTotalIndicators() {
	i.once.Do(func() {
		forever.Run(time.Second, func() {
			epoch, err := i.GetLatestTipset(context.Background())
			if err != nil {
				log.Errorf("call Get Latest Tipset error: %s", err)
				return
			}
			if epoch != nil ||
				i.totalIndicators == nil ||
				epoch[0].ID > i.totalIndicators.TotalIndicators.LatestHeight ||
				time.Since(i.lastCacheTime) > 5*time.Second {
				var resp *filscan.TotalIndicatorsResponse
				resp, err = i.getTotalIndicators(context.Background(), filscan.TotalIndicatorsRequest{})
				if err != nil {
					log.Errorf("call getTotalIndicators error: %s", err)
				} else {
					i.totalIndicators = resp
					i.lastCacheTime = time.Now()
				}
			}
		})
	})
}

func (i *IndexBiz) BannerIndicator(ctx context.Context, req struct{}) (resp *filscan.BannerIndicatorsResponse, err error) {
	minerCount, err := i.bannerRepo.GetMinerPowerProportion(ctx)
	if err != nil {
		return
	}
	totalBalance, err := i.bannerRepo.GetTotalBalance(ctx)
	if err != nil {
		return
	}
	resp = &filscan.BannerIndicatorsResponse{}
	resp.TotalBalance = totalBalance
	totalPower := minerCount[0].QualityAdjPower.Add(minerCount[1].QualityAdjPower)
	if !totalPower.IsZero() {
		div32 := minerCount[0].QualityAdjPower.Div(totalPower)
		resp.Proportion32G = &div32
		div64 := minerCount[1].QualityAdjPower.Div(totalPower)
		resp.Proportion64G = &div64
	}
	//如果总算力为空，则32G和64G算力的比都为0
	return
}

func (i *IndexBiz) TotalIndicators(ctx context.Context, req filscan.TotalIndicatorsRequest) (resp *filscan.TotalIndicatorsResponse, err error) {
	if i.totalIndicators != nil {
		resp = i.totalIndicators
		return
	}
	resp, err = i.getTotalIndicators(ctx, req)
	if err != nil {
		return
	}

	return
}

func (i *IndexBiz) getTotalIndicators(ctx context.Context, req filscan.TotalIndicatorsRequest) (resp *filscan.TotalIndicatorsResponse, err error) {
	// 获取最新区块高度
	latestTipset, err := i.GetLatestTipset(ctx)
	if err != nil {
		log.Errorf("latestTipset: %s", err.Error())
	}
	var epoch chain.Epoch
	if latestTipset != nil {
		epoch = chain.Epoch(latestTipset[0].ID)
	}

	// 当前基础费率
	var latestEpoch *londobell.EpochReply
	latestEpoch, err = i.GetEpoch(ctx, epoch)
	if err != nil {
		log.Errorf("latestEpoch: %s", err.Error())
	}
	var blockTime int64
	var baseFee decimal.Decimal
	if latestEpoch != nil {
		blockTime = epoch.Time().Unix()
		baseFee = latestEpoch.BaseFee
	}

	//log.Debugf("latest tipset height: %d(%s)", epoch, epoch.Format())

	// 获取当前扇区质押量
	var initialPledge decimal.Decimal
	initialPledge, err = i.GetInitialPledge(ctx)
	if err != nil {
		log.Errorf("initialPledge: %s", err.Error())
	}

	// 获取全网有效算力
	var netPower acl.NetPower
	netPower, err = i.GetTotalQualityPower(ctx, epoch)
	if err != nil {
		log.Errorf("totalQualityPower: %s", err.Error())
	}

	// 获取全网出块奖励
	var totalRewards decimal.Decimal
	totalRewards, err = i.GetTotalRewards(ctx, epoch)
	if err != nil {
		log.Errorf("totalRewards: %s", err.Error())
	}
	// 获取近24h增长算力
	var powerIncrease24H decimal.Decimal
	powerIncrease24H, err = i.GetPowerIncrease24H(ctx, epoch)
	if err != nil {
		log.Errorf("powerIncrease24H: %s", err.Error())
	}

	// 获取近24h出块奖励
	var rewardIncrease24H decimal.Decimal
	rewardIncrease24H, err = i.GetRewardIncrease24H(ctx, epoch)
	if err != nil {
		log.Errorf("rewardIncrease24H: %s", err.Error())
		rewardIncrease24H = decimal.Zero
	}

	// 获取近24h产出效率
	var rewardEfficiency24H decimal.Decimal
	if !netPower.QualityPower.IsZero() {
		rewardEfficiency24H = rewardIncrease24H.Div(netPower.QualityPower).Mul(decimal.NewFromInt(1024).Pow(decimal.NewFromInt(4)))
	}

	// 获取每赢票奖励
	var winCountReward decimal.Decimal
	winCountReward, err = i.GetWinCountReward(ctx, epoch)
	if err != nil {
		log.Errorf("winCountReward: %s", err.Error())
	}

	// 获取近24h平均每高度区块数
	var avgBlockCount decimal.Decimal
	avgBlockCount, err = i.GetAvgBlockCount(ctx, epoch)
	if err != nil {
		log.Errorf("avgBlockCount: %s", err.Error())
	}

	// 获取近24h平均每高度消息数
	var avgMessageCount float64
	{
		var totalBlocksCount decimal.Decimal
		totalBlocksCount, err = i.messageCountDal.GetAvgBlockCount24h(ctx)
		if err != nil {
			log.Errorf("avgMessageCount: %s", err.Error())
			totalBlocksCount = decimal.Zero
		}
		avgMessageCount, _ = totalBlocksCount.Float64()
		//var totalMessageCount int64
		//totalMessageCount, err = i.CountOfBlockMessages(ctx, epoch-2880, epoch)
		//if err != nil {
		//	log.Errorf("get CountOfBlockMessages error: %s", err)
		//}
		//avgMessageCount = float64(totalMessageCount) / 2880
	}

	// 获取活跃节点数
	var activeMiner int64
	activeMiner, err = i.GetActiveMiners(ctx, epoch)
	if err != nil {
		log.Errorf("activeMiner: %s", err.Error())
	}

	// 获取销毁量
	var burnt decimal.Decimal
	burnt, err = i.GetBurnt(ctx, epoch)
	if err != nil {
		log.Errorf("burnt: %s", err.Error())
	}

	// 获取流通率
	var circulatingPercent decimal.Decimal
	circulatingPercent, err = i.GetCirculatingPercent(ctx)
	if err != nil {
		log.Errorf("circulatingPercent: %s", err.Error())
	}

	gasCost, err := i.baseFeeTrendDal.GetGasPerT(ctx)
	if err != nil {
		log.Errorf("gasCost: %s", err.Error())
	}
	var gasCost32G decimal.Decimal
	var addPower32G decimal.Decimal
	var gasCost64G decimal.Decimal
	var addPower64G decimal.Decimal
	if gasCost != nil {
		gasCost32G = gasCost.Gas32G
		addPower32G = gasCost.Gas32G.Add(initialPledge)
		gasCost64G = gasCost.Gas64G
		addPower64G = gasCost.Gas64G.Add(initialPledge)
	}

	latest, err := i.gasTrendRepo.GetLatestMethodGasCostEpoch(ctx)
	if err != nil {
		log.Errorf("get latest epoch failed: %s", err.Error())
		latest = chain.Epoch(0)
	}
	entities, err := i.gasTrendRepo.GetMethodGasFees(ctx, chain.NewLCRORange(latest.Next()-2880, latest.Next()))
	if err != nil {
		log.Errorf("get method gas fees failed: %s", err.Error())
	}
	sum := decimal.Zero
	contractGas := decimal.Zero
	for i := range entities {
		sum = sum.Add(entities[i].GasCost)
		if entities[i].Method == "InvokeContract" || entities[i].Method == "CreateExternal" {
			contractGas = contractGas.Add(entities[i].GasCost)
		}
	}

	resp = &filscan.TotalIndicatorsResponse{}
	resp.TotalIndicators = filscan.TotalIndicators{
		LatestHeight:       epoch.Int64(),
		LatestBlockTime:    blockTime,
		TotalRewards:       totalRewards,
		TotalQualityPower:  netPower.QualityPower,
		BaseFee:            baseFee,
		MinerInitialPledge: initialPledge,
		PowerIncrease24H:   powerIncrease24H,
		RewardsIncrease24H: rewardIncrease24H,
		FilPerTera24H:      rewardEfficiency24H,
		WinCountReward:     winCountReward,
		AvgBlockCount:      avgBlockCount,
		AvgMessageCount:    avgMessageCount,
		ActiveMiners:       activeMiner,
		Burnt:              burnt,
		CirculatingPercent: circulatingPercent,
		GasIn32G:           gasCost32G,
		AddPowerIn32G:      addPower32G,
		GasIn64G:           gasCost64G,
		AddPowerIn64G:      addPower64G,
		Sum:                sum,
		ContractGas:        contractGas,
		Others:             sum.Sub(contractGas),
	}

	resp.TotalIndicators.Dc = netPower.QualityPower.Sub(netPower.RawBytePower).Div(decimal.NewFromInt(9))
	resp.TotalIndicators.Cc = netPower.RawBytePower.Sub(resp.TotalIndicators.Dc)

	return
}

func (i *IndexBiz) SearchInfo(ctx context.Context, req filscan.SearchInfoRequest) (resp filscan.SearchInfoResponse, err error) {
	input := strings.TrimSpace(req.Input)
	if chain.SmartAddress(input).IsValid() || regexp.MustCompile("^0x[0-9a-fA-F]{40}$").MatchString(input) || req.InputType.Value() == types.ADDRESS {
		if regexp.MustCompile("^0x[0-9a-fA-F]{40}$").MatchString(input) {
			input, err = CheckETHAddress(input)
			if err != nil {
				return
			}
		}
		var inputAccount chain.SmartAddress
		if input != "" {
			inputAccount = chain.SmartAddress(input)
		} else {
			return
		}
		var accountDetails *filscan.AccountBasic
		var epoch chain.Epoch
		accountDetails, epoch, err = i.accountBasicByID(ctx, inputAccount)
		if err != nil {
			return
		}
		resp.Epoch = epoch.Int64()
		if accountDetails != nil {
			if accountDetails.AccountType == types.MINER {
				resp.ResultType = types.MINER
			} else {
				resp.ResultType = types.ADDRESS
			}
		} else {
			resp.ResultType = types.ADDRESS
		}
	}
	if regexp.MustCompile("^bafy2bzace").MatchString(input) || regexp.MustCompile("^0x[0-9a-fA-F]{64}$").MatchString(input) || req.InputType.Value() == types.CID {
		if regexp.MustCompile("^0x[0-9a-fA-F]{64}$").MatchString(input) {
			var messagePool *filscan.MessagesPoolResponse
			messagePool, err = i.GetMessagePool(ctx, input, nil)
			if err != nil {
				return
			}
			if messagePool != nil && messagePool.MessagesPoolList != nil {
				input = messagePool.MessagesPoolList[0].MessageBasic.Cid
			} else {
				var messageCid string
				messageCid, err = i.GetMessageCidByHash(ctx, input)
				if err != nil {
					return
				}
				if messageCid != "" {
					input = messageCid
				} else {
					return
				}
			}

		}
		//var messageDetails *filscan.MessageDetails
		_, err = i.GetMessageDetails(ctx, input) //nolint
		if err == nil {
			resp.ResultType = "message_details"
			return resp, nil
		} else {

			var blockDetails *filscan.BlockDetails
			blockDetails, err = i.GetBlockDetails(ctx, input)
			if err != nil {
				return
			}
			if blockDetails != nil {
				resp.ResultType = "block_details"
			}
		}
	}
	if regexp.MustCompile("^[1-9]\\d*$").MatchString(input) || req.InputType.Value() == types.HEIGHT {
		var latestTipset []*londobell.Tipset
		latestTipset, err = i.GetLatestTipset(ctx)
		if err != nil {
			return
		}
		var inputNum int64
		inputNum, err = strconv.ParseInt(input, 10, 64)
		if err != nil {
			return
		}
		if inputNum <= latestTipset[0].ID {
			resp.ResultType = types.HEIGHT
		}
	}
	if strings.HasSuffix(strings.ToLower(input), ".fil") {
		a := strings.ToLower(input)
		var items []*po.FNSToken
		items, err = i.SearchFnsTokens(ctx, a)
		if err != nil {
			return
		}
		if len(items) == 0 {
			return
		}
		resp.ResultType = types.FNS
		for _, vv := range items {
			p := providers.GetProvider(vv.Provider)
			resp.FNSTokens = append(resp.FNSTokens, &filscan.SearchFNSToken{
				Name:     vv.Name,
				Provider: providers.ToContract(vv.Provider),
				Icon:     p.LOGO,
			})
		}
		return
	}

	// 返回404
	return
}
