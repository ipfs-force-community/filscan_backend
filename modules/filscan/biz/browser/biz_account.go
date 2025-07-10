package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"

	"github.com/ethereum/go-ethereum/common"
	goAddress "github.com/filecoin-project/go-address"
	lotus "github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/gozelle/mix"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/debuglog"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

func NewAccountBiz(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB, config *config.Config) *AccountBiz {
	return &AccountBiz{
		AccountAclImpl: acl.NewAccountAclImpl(agg, adapter),
		MinerInfoBiz:   NewMinerInfoBiz(db, agg, adapter),
		OwnerInfoBiz:   NewOwnerInfoBiz(db),
		ActorInfoBiz:   NewActorInfoBizBiz(db),
		Redis:          redis.NewRedis(config),
		evmTransfer:    dal.NewEVMTransferDal(db),
		evmContract:    dal.NewEvmContractDal(db),
		repo:           prodal.NewMinerInfoDal(db),
	}
}

var _ filscan.AccountAPI = (*AccountBiz)(nil)

type AccountBiz struct {
	*acl.AccountAclImpl
	*MinerInfoBiz
	*OwnerInfoBiz
	*ActorInfoBiz
	*redis.Redis
	evmTransfer *dal.EVMTransferDal
	evmContract *dal.EvmContractDal
	repo        prorepo.MinerRepo
}

func (a AccountBiz) TransferMethodByAccountID(ctx context.Context, req filscan.AllMethodByAccountIDRequest) (resp filscan.AllTransferMethodByAccountIDResponse, err error) {
	if req.AccountID == "f090" || req.AccountID == "f0121" || req.AccountID == "f0118" || req.AccountID == "f0120" ||
		req.AccountID == "f0119" || req.AccountID == "f0117" {
		resp.MethodNameList = []string{"Transfer", "Genesis"}
		return
	}
	resp.MethodNameList = append(resp.MethodNameList, "Send", "Receive", "Transfer", "Burn")
	if req.AccountID[0] == '0' && req.AccountID[1] == 'x' {
		return resp, nil
	}
	actorInfo, err := a.MinerInfoBiz.adapter.Actor(ctx, chain.SmartAddress(req.AccountID), nil)
	if err != nil {
		return
	}
	if actorInfo.ActorType == types.MINER {
		resp.MethodNameList = append(resp.MethodNameList, "Blockreward")
	}

	return resp, nil
}

func (a AccountBiz) accountBasicByID(ctx context.Context, address chain.SmartAddress) (accountBasic *filscan.AccountBasic, epoch chain.Epoch, err error) {
	epoch, err = a.GetHeight(ctx)
	if err != nil {
		return
	}
	accountBasic, err = a.GetAccountBasic(ctx, address, epoch)
	if err != nil {
		return
	}

	if accountBasic == nil {
		if address.IsValid() {
			accountBasic = &filscan.AccountBasic{
				AccountAddress: address.Address(),
				AccountType:    types.TOBECREATED,
			}
			var ethAddress string
			ethAddress, err = TransferToETHAddress(address.Address())
			if err != nil {
				return
			}
			if ethAddress != "" {
				accountBasic.EthAddress = ethAddress
			}
			return
		}
		return
	}

	if (accountBasic.AccountAddress == types.EMPTY && accountBasic.AccountType != "miner") ||
		accountBasic.AccountType == types.EVM ||
		accountBasic.AccountType == types.ETHACCOUNT ||
		accountBasic.AccountType == types.PLACEHOLDER {
		var actorAddress *londobell.Address
		actorAddress, err = a.CheckActorAddress(ctx, chain.SmartAddress(accountBasic.AccountID))
		if err != nil && !strings.Contains(err.Error(), "not found") {
			return
		}

		if actorAddress == nil {
			actorAddress = &londobell.Address{
				RobustAddress: accountBasic.AccountAddress,
			}
		}
		if accountBasic.AccountType == types.EVM ||
			accountBasic.AccountType == types.ETHACCOUNT ||
			accountBasic.AccountType == types.PLACEHOLDER {
			if actorAddress.RobustAddress != accountBasic.AccountAddress {
				accountBasic.StableAddress = chain.SmartAddress(actorAddress.RobustAddress).Address()
			}
			var ethAddress string
			ethAddress, err = TransferToETHAddress(accountBasic.AccountAddress)
			if err != nil {
				return
			}
			accountBasic.EthAddress = ethAddress
		} else {
			accountBasic.AccountAddress = chain.SmartAddress(actorAddress.RobustAddress).Address()
			accountBasic.EthAddress, err = TransferToETHAddress(accountBasic.AccountID)
			if err != nil {
				return nil, 0, err
			}
		}
	} else {
		accountBasic.EthAddress, err = TransferToETHAddress(accountBasic.AccountID)
		if err != nil {
			return nil, 0, err
		}
	}
	accountID := chain.SmartAddress(accountBasic.AccountID)
	var actorInfo *bo.ActorInfo
	actorInfo, err = a.ActorInfoBiz.GetActorInfoOrNil(ctx, accountID)
	if err != nil && !strings.Contains(err.Error(), "error not found") {
		return
	}
	if actorInfo != nil {
		// 此处数据库设计缺陷，没有能正确处理时区，故此处修正
		if actorInfo.CreatedTime != nil {
			t, _ := chain.BuildEpochByTime(actorInfo.CreatedTime.Format("2006-01-02 15:04:05"))
			t1 := t.Unix()

			accountBasic.CreateTime = &t1
		}
		if actorInfo.LastTxTime != nil {
			t, _ := chain.BuildEpochByTime(actorInfo.LastTxTime.Format("2006-01-02 15:04:05"))
			t2 := t.Unix()
			accountBasic.LatestTransferTime = &t2
		}
	}

	var isOwner bool
	isOwner, err = a.OwnerInfoBiz.CheckIsOwner(ctx, accountID)
	if err != nil {
		return
	}
	if isOwner {
		var ownerInfo *filscan.AccountOwner
		ownerInfo, err = a.OwnerInfoBiz.GetOwnerInfo(ctx, accountID)
		if err != nil {
			return
		}
		accountBasic.ActiveMiners = ownerInfo.OwnedMiners
		accountBasic.OwnedMiners = ownerInfo.OwnedMiners
		filscanInfos, err := GetOwnMinersFromOldFilscan(ctx, accountID.Address())
		if err != nil {
			log.Errorf("get info from old filscan failed: %w", err)
		} else if filscanInfos != nil {
			accountBasic.OwnedMiners = filscanInfos.Result.Miners
		}
	}
	if accountBasic.CreateTime == nil || accountBasic.LatestTransferTime == nil {
		info, err := GetInfoFromFilfox(ctx, accountBasic.AccountID)
		if err != nil {
			log.Errorf("get info from filfox failed %w", err)
		} else {
			accountBasic.LatestTransferTime = &info.LastSeen
			accountBasic.CreateTime = &info.CreateTimestamp
		}
	}
	return
}

func (a AccountBiz) AccountInfoByID(ctx context.Context, req filscan.AccountInfoByIDRequest) (resp filscan.AccountInfoByIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	tempFilAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if tempFilAddress != "" {
		filAddress = chain.SmartAddress(tempFilAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	if accountBasic == nil {
		err = mix.Warnf("actor: %s not found", filAddress)
		return
	}

	accountID := chain.SmartAddress(accountBasic.AccountID)
	var accountType types.AccountType
	if req.Filters != nil {
		accountType = *req.Filters.AccountType
	} else {
		newAccountType := types.AccountType(accountBasic.AccountType)
		accountType = newAccountType
	}
	switch accountType.Value() {
	case types.ACCOUNT:
		resp.AccountInfo.AccountBasic = accountBasic
		resp.AccountType = types.ACCOUNT
	case types.MINER:
		var minerInfo *filscan.AccountMiner
		minerInfo, err = a.accountMinerByID(ctx, accountBasic)
		if err != nil {
			return
		}
		if minerInfo != nil {
			minerInfo.AccountBasic = accountBasic
			resp.AccountInfo.AccountMiner = minerInfo
			resp.AccountType = types.MINER
		}
	case types.MULTISIG:
		var multiSig *filscan.AccountMultisig
		multiSig, err = a.GetMultiSignBasic(ctx, accountID)
		if err != nil {
			return
		}
		if multiSig != nil {
			multiSig.AccountBasic = accountBasic
			resp.AccountInfo.AccountMultisig = multiSig
			resp.AccountType = types.MULTISIG
		}
	case types.EVM:
		resp.AccountInfo.AccountBasic = accountBasic
		resp.AccountType = types.EVM
		var evmTransferStats *po.EvmTransferStat
		evmTransferStats, err = a.evmTransfer.GetEvmTransferStatsByID(ctx, accountID.Address())
		if err != nil {
			return
		}
		if evmTransferStats != nil {
			contract := assembler.EvmContract{}.EvmTransferStatsToContract(evmTransferStats)
			resp.AccountInfo.AccountBasic.EvmContract = contract
		} else {
			var evmContractSol *po.FEvmContractSols
			evmContractSol, err = a.evmContract.SelectFEvmMainContractByActorID(ctx, accountID.Address())
			if err != nil {
				return
			}
			if evmContractSol != nil {
				evmContract := &filscan.EvmContract{
					ActorID:      accountBasic.AccountID,
					ContractName: evmContractSol.ContractName,
				}
				resp.AccountInfo.AccountBasic.EvmContract = evmContract
			}
		}
	case types.ETHACCOUNT:
		resp.AccountInfo.AccountBasic = accountBasic
		resp.AccountType = types.ETHACCOUNT
	case types.PLACEHOLDER:
		resp.AccountInfo.AccountBasic = accountBasic
		resp.AccountType = types.PLACEHOLDER
	default:
		resp.AccountInfo.AccountBasic = accountBasic
		resp.AccountType = accountBasic.AccountType
	}

	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) accountMinerByID(ctx context.Context, accountBasic *filscan.AccountBasic) (accountMiner *filscan.AccountMiner, err error) {
	accountID := chain.SmartAddress(accountBasic.AccountID)
	accountMinerInfo, err := a.GetAccountMinerInfo(ctx, accountID)
	if err != nil {
		return
	}
	minerInfo, err := a.MinerInfoBiz.GetMinerInfo(ctx, accountID)
	if err != nil {
		return
	}
	ipAddress, err := a.MinerInfoBiz.GetMinerIpAddress(ctx, accountID)
	if err != nil {
		return
	}

	if accountMinerInfo != nil {
		accountMinerInfo.AccountBasic.AccountAddress = accountBasic.AccountAddress
		accountMinerInfo.AccountBasic.AccountType = accountBasic.AccountType
		accountMinerInfo.AccountBasic.MessageCount = accountBasic.MessageCount
		accountMinerInfo.AccountBasic.LatestTransferTime = accountBasic.LatestTransferTime
		accountMinerInfo.AccountBasic.CreateTime = accountBasic.CreateTime
		accountMinerInfo.AccountBasic.Nonce = accountBasic.Nonce
		accountMinerInfo.AccountBasic.CodeCid = accountBasic.CodeCid
	}
	if minerInfo != nil {
		accountMinerInfo.AccountIndicator.QualityPowerRank = minerInfo.AccountIndicator.QualityPowerRank
		accountMinerInfo.AccountIndicator.QualityPowerPercentage = minerInfo.AccountIndicator.QualityPowerPercentage
		accountMinerInfo.AccountIndicator.TotalBlockCount = minerInfo.AccountIndicator.TotalBlockCount
		accountMinerInfo.AccountIndicator.TotalWinCount = minerInfo.AccountIndicator.TotalWinCount
		accountMinerInfo.AccountIndicator.TotalReward = minerInfo.AccountIndicator.TotalReward
	}
	accountMinerInfo.IpAddress = ipAddress
	accountMiner = accountMinerInfo
	return
}

type OldFilscanOwnerInfos struct {
	Result struct {
		Miners []string `json:"miners"`
	} `json:"result"`
}

func GetOwnMinersFromOldFilscan(ctx context.Context, addr string) (*OldFilscanOwnerInfos, error) {
	req := struct {
		Id      int      `json:"id"`
		JsonRpc string   `json:"jsonrpc"`
		Params  []string `json:"params"`
		Method  string   `json:"method"`
	}{
		Id:      1,
		JsonRpc: "2.0",
		Params:  []string{addr},
		Method:  "filscan.FilscanActorById",
	}

	bs, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	reqs, _ := http.NewRequest("POST", "https://api.filscan.io:8700/rpc/v1", bytes.NewBuffer(bs))
	resp, err := client.Do(reqs)
	if err != nil {
		return nil, err
	}
	bs, err = io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	res := OldFilscanOwnerInfos{}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (a AccountBiz) AccountOwnerByID(ctx context.Context, req filscan.AccountOwnerByIDRequest) (resp filscan.AccountOwnerByIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	tempFilAddress, err := CheckETHAddress(req.OwnerID)
	if err != nil {
		return
	}
	if tempFilAddress != "" {
		filAddress = chain.SmartAddress(tempFilAddress)
	} else {
		filAddress = chain.SmartAddress(req.OwnerID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	accountID := chain.SmartAddress(accountBasic.AccountID)
	if accountBasic.OwnedMiners != nil {
		var OwnedMiners []chain.SmartAddress
		for _, miner := range accountBasic.OwnedMiners {
			OwnedMiners = append(OwnedMiners, chain.SmartAddress(miner))
		}
		var ownedMinersInfo []*filscan.AccountMiner
		ownedMinersInfo, err = a.GetOwnedMinersInfo(ctx, OwnedMiners)
		if err != nil {
			return
		}
		var convertor assembler.ActorInfo
		ownerIndicator := convertor.OwnedMinersToOwnerIndicator(ownedMinersInfo)

		var ownerInfo *filscan.AccountOwner
		ownerInfo, err = a.OwnerInfoBiz.GetOwnerInfo(ctx, accountID)
		if err != nil {
			return
		}
		infos, err := GetOwnMinersFromOldFilscan(ctx, accountID.Address())
		if err != nil {
			log.Errorf("get info from old filscan failed: %w", err)
		}
		if ownerIndicator != nil && ownerInfo != nil {
			ownerInfo.AccountID = accountBasic.AccountID
			ownerInfo.AccountAddress = accountBasic.AccountAddress
			ownerInfo.ActiveMiners = accountBasic.OwnedMiners
			if infos != nil {
				ownerInfo.OwnedMiners = infos.Result.Miners
			}
			ownerIndicator.AccountID = accountBasic.AccountID
			ownerIndicator.QualityPowerRank = ownerInfo.AccountIndicator.QualityPowerRank
			ownerIndicator.QualityPowerPercentage = ownerInfo.AccountIndicator.QualityPowerPercentage
			ownerIndicator.TotalBlockCount = ownerInfo.AccountIndicator.TotalBlockCount
			ownerIndicator.TotalWinCount = ownerInfo.AccountIndicator.TotalWinCount
			ownerIndicator.TotalReward = ownerInfo.AccountIndicator.TotalReward
			ownerInfo.AccountIndicator = ownerIndicator
			resp.AccountOwner = ownerInfo
		}
	}
	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) IndicatorsByAccountID(ctx context.Context, req filscan.IndicatorsByAccountIDRequest) (resp filscan.IndicatorsByAccountIDResponse, err error) {
	defer func() {
		debuglog.Logger.Info("resp", resp, err)
	}()
	debuglog.Logger.Infof("IndicatorsByAccountID req: %v", req)

	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	tempFilAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if tempFilAddress != "" {
		filAddress = chain.SmartAddress(tempFilAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	if accountBasic != nil {
		accountID := chain.SmartAddress(accountBasic.AccountID)
		var accountType *types.AccountType
		if req.Filters.AccountType != nil {
			accountType = req.Filters.AccountType
		} else {
			newAccountType := types.AccountType(accountBasic.AccountType)
			accountType = &newAccountType
		}
		var accountInterval *types.IntervalType
		if req.Filters.Interval != nil {
			accountInterval = req.Filters.Interval
		}
		if accountBasic.OwnedMiners != nil {
			var ownerIndicator *filscan.MinerIndicators
			ownerIndicator, err = a.OwnerInfoBiz.GetOwnerIndicator(ctx, accountID, accountInterval)
			if err != nil {
				return
			}
			resp.MinerIndicators = ownerIndicator
		} else if accountType.Value() == types.MINER {
			var minerIndicator *filscan.MinerIndicators
			minerIndicator, err = a.MinerInfoBiz.GetMinerIndicator(ctx, accountID, accountInterval)
			if err == nil {
				resp.MinerIndicators = minerIndicator
				//todo 这里有两种方式，采用商务面板的方式存在延迟，因此使用更实时根据每个epoch累加计算方式
			} else {
				log.Errorf("get miner indicator failed: %w", err)
				if strings.Contains(err.Error(), "actor not found") {
					resp.MinerIndicators = &filscan.MinerIndicators{}
					return resp, nil
				}
			}

		}
	}
	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}

	return
}

func (a AccountBiz) getWindowPoStGas(ctx context.Context, addr string, interval *types.IntervalType) (result decimal.Decimal, err error) {
	endEpoch, err := a.repo.GetSyncEpoch(ctx)
	if err != nil {
		return
	}
	var startEpoch chain.Epoch
	switch interval.Value() {
	case types.DAY:
		startEpoch = chain.Epoch(endEpoch - 2880)
	case types.WEEK:
		startEpoch = chain.Epoch(endEpoch - 2880*7)
	case types.MONTH:
		startEpoch = chain.Epoch(endEpoch - 2880*30)
	}
	fees, err := a.repo.GetMinerFunds(ctx, chain.NewLORCRange(startEpoch+120, chain.Epoch(endEpoch)+120), []string{addr})
	if err != nil {
		return
	}
	for _, fund := range fees {
		result = fund.WdPostGas
	}
	return
}

func (a AccountBiz) BalanceTrendByAccountID(ctx context.Context, req filscan.BalanceTrendByAccountIDRequest) (resp filscan.BalanceTrendByAccountIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	tempFilAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if tempFilAddress != "" {
		filAddress = chain.SmartAddress(tempFilAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	if accountBasic != nil && accountBasic.AccountType == types.TOBECREATED {
		return
	}
	accountID := chain.SmartAddress(accountBasic.AccountID)
	var accountType *types.AccountType
	if req.Filters.AccountType != nil {
		accountType = req.Filters.AccountType
	} else {
		newAccountType := types.AccountType(accountBasic.AccountType)
		accountType = &newAccountType
	}
	var accountInterval *types.IntervalType
	if req.Filters.Interval != nil {
		accountInterval = req.Filters.Interval
	} else {
		ai := types.IntervalType(types.MONTH)
		accountInterval = &ai
	}
	var accountBalanceTrend []*filscan.BalanceTrend
	switch accountType.Value() {
	case types.OWNER:
		accountBalanceTrend, err = a.OwnerInfoBiz.OwnerBalanceTrend(ctx, accountID, accountInterval.Value())
		if err != nil {
			return
		}
		resp.BalanceTrendByAccountIDList = accountBalanceTrend
	case types.MINER:
		var epoch *int64
		epoch, err = a.MinerInfoBiz.MinerEpoch(ctx)
		if err != nil {
			return
		}
		accountBalanceTrend, err = a.ActorInfoBiz.GetActorBalanceTrend(ctx, accountBasic, accountInterval.Value(), epoch)
		if err != nil {
			return
		}
		for _, v := range accountBalanceTrend {
			v.Epoch = v.Height.Int64()
		}
		accountBalanceTrend, err = a.MinerInfoBiz.MinerBalanceTrend(ctx, accountID, accountInterval.Value(), epoch, accountBalanceTrend)
		if err != nil {
			return
		}
		resp.BalanceTrendByAccountIDList = accountBalanceTrend
	default:
		accountBalanceTrend, err = a.ActorInfoBiz.GetActorBalanceTrend(ctx, accountBasic, accountInterval.Value(), nil)
		if err != nil {
			return
		}
		resp.BalanceTrendByAccountIDList = accountBalanceTrend
	}
	sort.Slice(resp.BalanceTrendByAccountIDList, func(i, j int) bool {
		return resp.BalanceTrendByAccountIDList[i].BlockTime < resp.BalanceTrendByAccountIDList[j].BlockTime
	})
	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) PowerTrendByAccountID(ctx context.Context, req filscan.PowerTrendByAccountIDRequest) (resp filscan.PowerTrendByAccountIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	tempFilAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if tempFilAddress != "" {
		filAddress = chain.SmartAddress(tempFilAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	accountID := chain.SmartAddress(accountBasic.AccountID)
	var accountType *types.AccountType
	if req.Filters.AccountType != nil {
		accountType = req.Filters.AccountType
	} else {
		newAccountType := types.AccountType(accountBasic.AccountType)
		accountType = &newAccountType
	}
	var accountInterval *types.IntervalType
	if req.Filters.Interval != nil {
		accountInterval = req.Filters.Interval
	}
	switch accountType.Value() {
	case types.OWNER:
		var ownerPowerTrend []*filscan.PowerTrend
		ownerPowerTrend, err = a.OwnerInfoBiz.OwnerPowerTrend(ctx, accountID, accountInterval.Value())
		if err != nil {
			return
		}
		resp.PowerTrendByAccountIDList = ownerPowerTrend
	case types.MINER:
		var minerPowerTrend []*filscan.PowerTrend
		minerPowerTrend, err = a.MinerInfoBiz.MinerPowerTrend(ctx, accountID, accountInterval.Value())
		if err != nil {
			return
		}
		resp.PowerTrendByAccountIDList = minerPowerTrend
	}
	sort.Slice(resp.PowerTrendByAccountIDList, func(i, j int) bool {
		return resp.PowerTrendByAccountIDList[i].BlockTime < resp.PowerTrendByAccountIDList[j].BlockTime
	})

	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) BlocksByAccountID(ctx context.Context, req filscan.BlocksByAccountIDRequest) (resp filscan.BlocksByAccountIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	tempFilAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if tempFilAddress != "" {
		filAddress = chain.SmartAddress(tempFilAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	actorBlocks, err := a.GetActorBlocks(ctx, filAddress, req.Filters)
	if err != nil {
		return
	}
	if actorBlocks != nil {
		resp = *actorBlocks
	}
	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) MessagesByAccountID(ctx context.Context, req filscan.MessagesByAccountIDRequest) (resp filscan.MessagesByAccountIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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

	var filAddress chain.SmartAddress
	ethAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if ethAddress != "" {
		filAddress = chain.SmartAddress(ethAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	if accountBasic != nil && accountBasic.AccountType == types.TOBECREATED {
		return
	}
	actorMessages, err := a.GetActorMessages(ctx, filAddress, req.Filters)
	if err != nil {
		return
	}
	if actorMessages != nil {
		resp = *actorMessages
	}

	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) PendingMsgByAccount(ctx context.Context, req filscan.PendingMsgByAccountRequest) (resp filscan.MessagesPoolResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var pendingMessageList *londobell.MessagePool
	pendingMessageList, err = a.MinerInfoBiz.agg.MessagePool(ctx, "", &types.Filters{})
	if err != nil {
		return
	}
	resp = getAccountPending(req, pendingMessageList)

	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func getAccountPending(ac filscan.PendingMsgByAccountRequest, pool *londobell.MessagePool) filscan.MessagesPoolResponse {
	var (
		resp    filscan.MessagesPoolResponse
		convert assembler.BlockChainInfo
	)

	for _, msg := range pool.PendingMessage {
		if msg.From.Address() == ac.AccountID || msg.To.Address() == ac.AccountID || msg.From.Address() == ac.AccountAddr || msg.To.Address() == ac.AccountAddr {
			messagePool := convert.PendingMessageToMessagePool(msg)
			resp.MessagesPoolList = append(resp.MessagesPoolList, messagePool)
			resp.TotalCount++

		}
	}
	return resp
}

type FilfoxInfo struct {
	CreateTimestamp int64 `json:"createTimestamp"`
	LastSeen        int64 `json:"lastSeenTimestamp"`
}

func GetInfoFromFilfox(ctx context.Context, addr string) (FilfoxInfo, error) {
	res, err := http.Get(fmt.Sprintf("https://filfox.info/api/v1/address/%s", addr))
	if err != nil {
		return FilfoxInfo{}, err
	}
	bs, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return FilfoxInfo{}, err
	}

	reply := FilfoxInfo{}
	err = json.Unmarshal(bs, &reply)
	if err != nil {
		return FilfoxInfo{}, err
	}

	return reply, err
}

type FilfoxTraceItems struct {
	Height     int64           `json:"height"`
	Timestamp  int64           `json:"timestamp"`
	MessageCid string          `json:"message"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	Value      decimal.Decimal `json:"value"`
	Type       string          `json:"type"`
}

type FilfoxTraces struct {
	Transfers  []FilfoxTraceItems `json:"transfers"`
	TotalCount int64              `json:"totalCount"`
}

func FetchTracesByFilfox(ctx context.Context, addr, method string, limit, page int64) (*FilfoxTraces, error) {
	url := fmt.Sprintf("https://filfox.info/api/v1/address/%s/transfers?pageSize=%d&page=%d", addr, limit, page)
	if method != "All" && method != "" {
		url += fmt.Sprintf("&type=%s", strings.ToLower(method))
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	bs, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	res := &FilfoxTraces{}
	err = json.Unmarshal(bs, res)
	return res, err
}

func (a AccountBiz) TracesByAccountID(ctx context.Context, req filscan.TracesByAccountIDRequest) (resp filscan.TracesByAccountIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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
	var filAddress chain.SmartAddress
	var accountBasic *filscan.AccountBasic
	var epoch chain.Epoch
	var actorTransfers *filscan.TracesByAccountIDResponse
	var ethAddress string

	if req.AccountID == "f090" || req.AccountID == "f0121" || req.AccountID == "f0118" || req.AccountID == "f0120" ||
		req.AccountID == "f0119" || req.AccountID == "f0117" {
		res, err := FetchTracesByFilfox(ctx, req.AccountID, req.Filters.MethodName, req.Filters.Limit, req.Filters.Index)
		if err != nil {
			log.Errorf("get traces from filfox failed:%s %w", req.AccountID, err)
			goto Return
		}

		resp.TotalCount = res.TotalCount
		for i := range res.Transfers {
			transfer := res.Transfers[i]
			method := transfer.Type
			method = strings.ToUpper(method[0:1]) + method[1:]
			resp.TracesByAccountIDList = append(resp.TracesByAccountIDList, &filscan.MessageBasic{
				Height:     nil,
				BlockTime:  uint64(transfer.Timestamp),
				Cid:        transfer.MessageCid,
				From:       transfer.From,
				To:         transfer.To,
				Value:      transfer.Value,
				MethodName: method,
			})
		}
		goto Return
	}

	ethAddress, err = CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if ethAddress != "" {
		filAddress = chain.SmartAddress(ethAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err = a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}

	resp.Epoch = epoch.Int64()
	if accountBasic != nil && accountBasic.AccountType == types.TOBECREATED {
		return
	}
	if req.Filters.MethodName == "All" {
		req.Filters.MethodName = ""
	} else {
		req.Filters.MethodName = strings.ToLower(req.Filters.MethodName)
	}
	actorTransfers, err = a.GetActorTransfers(ctx, filAddress, req.Filters)
	if err != nil {
		return
	}
	if actorTransfers != nil {
		for i := range actorTransfers.TracesByAccountIDList {
			if actorTransfers.TracesByAccountIDList[i].MethodName == "Burn" ||
				actorTransfers.TracesByAccountIDList[i].MethodName == "Send" {
				actorTransfers.TracesByAccountIDList[i].Value = actorTransfers.TracesByAccountIDList[i].Value.Neg()
			}
		}
		resp = *actorTransfers
	}
Return:
	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func (a AccountBiz) AllMethodByAccountID(ctx context.Context, req filscan.AllMethodByAccountIDRequest) (resp filscan.AllMethodByAccountIDResponse, err error) {
	cacheKey, err := a.Redis.HexCacheKey(ctx, req)
	if err != nil {
		return
	}
	cacheResult, err := a.GetCacheResult(cacheKey)
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

	var filAddress chain.SmartAddress
	ethAddress, err := CheckETHAddress(req.AccountID)
	if err != nil {
		return
	}
	if ethAddress != "" {
		filAddress = chain.SmartAddress(ethAddress)
	} else {
		filAddress = chain.SmartAddress(req.AccountID)
	}
	accountBasic, epoch, err := a.accountBasicByID(ctx, filAddress)
	if err != nil {
		return
	}
	resp.Epoch = epoch.Int64()
	if accountBasic != nil && accountBasic.AccountType == types.TOBECREATED {
		return
	}
	methodNames, err := a.GetAllMethodNameByID(ctx, filAddress)
	if err != nil {
		return
	}
	if methodNames != nil {
		resp.MethodNameList = methodNames
	}

	err = a.Redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	return
}

func CheckETHAddress(inputAddress string) (outputFilAddress string, err error) {
	if regexp.MustCompile("^0x[0-9a-fA-F]{40}$").MatchString(inputAddress) {
		ethAddress := lotus.EthAddress(common.HexToAddress(inputAddress))
		var filecoinAddress goAddress.Address
		filecoinAddress, err = ethAddress.ToFilecoinAddress()
		if err != nil {
			return
		}
		if !filecoinAddress.Empty() && chain.SmartAddress(filecoinAddress.String()).IsValid() {
			outputFilAddress = filecoinAddress.String()
		}
	}
	return
}

func TransferToETHAddress(inputAddress string) (outputEthAddress string, err error) {
	var filecoinAddress goAddress.Address
	filecoinAddress, err = goAddress.NewFromString(chain.SmartAddress(inputAddress).Address())
	if err != nil {
		return
	}
	var ethAddress lotus.EthAddress
	ethAddress, err = lotus.EthAddressFromFilecoinAddress(filecoinAddress)
	if err != nil {
		return
	}
	outputEthAddress = ethAddress.String()
	return
}
