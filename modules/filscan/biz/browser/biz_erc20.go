package browser

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gozelle/async/parallel"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_ave"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/erc20"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/impl"
)

func NewERC20Biz(db *gorm.DB, adapter londobell.Adapter, agg londobell.Agg, abiDecoder fevm.ABIDecoderAPI) *ERC20Biz {
	e := &ERC20Biz{
		repo:    dal.NewERC20Dal(db),
		adapter: adapter,
		agg:     agg,
		abi:     abiDecoder,
	}
	go e.ERC20List(context.TODO(), struct{}{}) //nolint
	return e
}

type ERC20Biz struct {
	repo    repository.ERC20TokenRepo
	adapter londobell.Adapter
	agg     londobell.Agg
	abi     fevm.ABIDecoderAPI
}

func (e ERC20Biz) ERC20RecentTransfer(ctx context.Context, request *filscan.ERC20RecentTransferReq) (*filscan.ERC20TransferReply, error) {
	pastEpoch := 0
	switch request.Duration {
	case "1h":
		pastEpoch = 120
	case "1d":
		pastEpoch = 2880
	case "7d":
		pastEpoch = 2880 * 7
	case "30d":
		pastEpoch = 2880 * 30
	case "365d":
		pastEpoch = 2880 * 365
	default:
		return nil, fmt.Errorf("invalid duration, only 1h, 1d, 7d, 30d are permitted")
	}

	reply := &filscan.ERC20TransferReply{}
	h, err := e.agg.FinalHeight(ctx)
	if err != nil {
		return nil, fmt.Errorf("get chain head failed: %w", err)
	}

	total, dbRes, err := e.repo.GetERC20TransferBatchAfterEpochInOneContract(ctx, request.ContractID, int(h.Int64())-pastEpoch, int(request.Limit), int(request.Page))
	if err != nil {
		return nil, err
	}
	for i := range dbRes {
		reply.Items = append(reply.Items, &filscan.ERC20Transfer{
			Cid:       dbRes[i].Cid,
			Method:    dbRes[i].Method,
			Time:      chain.Epoch(dbRes[i].Epoch).Time(),
			From:      dbRes[i].From,
			To:        dbRes[i].To,
			Amount:    dbRes[i].Amount.Div(decimal.New(1, int32(dbRes[i].Decimal))),
			TokenName: dbRes[i].TokenName,
		})
	}
	sort.Slice(reply.Items, func(i, j int) bool {
		return reply.Items[i].Time.After(reply.Items[j].Time)
	})
	reply.Total = total
	return reply, nil
}

func (e ERC20Biz) ERC20AddrTransfers(ctx context.Context, request *filscan.ERC20AddrTransfersReq) (*filscan.ERC20AddrTransfersReply, error) {
	if len(request.Address) == 0 {
		return nil, nil
	}
	var err error
	addr := strings.ToLower(request.Address)
	if request.Address[0] == 'f' && request.Address[1] == '0' {
		addr, err = ToEthAddr(addr)
		if err != nil {
			return nil, err
		}
	} else if request.Address[0] == 'f' {
		actor, err := e.adapter.Actor(ctx, chain.SmartAddress(request.Address), nil)
		if err != nil {
			log.Errorf("get actor %s info failed, %w", request.Address, err)
			return nil, nil
		}

		if actor.DelegatedAddr != "" && actor.DelegatedAddr != "<empty>" {
			addr, err = ToEthAddr(actor.DelegatedAddr)
		} else {
			addr, err = ToEthAddr(actor.ActorID)
		}
		if err != nil {
			return nil, err
		}
	}

	total, dbRes, err := e.repo.GetERC20TransferByRelatedAddr(ctx, addr, request.TokenName, int(request.Filters.Page), int(request.Filters.Limit))
	if err != nil {
		return nil, err
	}

	contracts := []string{}
	for i := range dbRes {
		contracts = append(contracts, dbRes[i].ContractId)
	}

	ps, err := e.repo.GetContractsUrl(ctx, contracts)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	mp := map[string]string{}
	for i := range ps {
		mp[ps[i].ContractId] = ps[i].Url
	}

	reply := []*filscan.ERC20Transfer{}

	for i := range dbRes {
		reply = append(reply, &filscan.ERC20Transfer{
			Cid:        dbRes[i].Cid,
			Method:     dbRes[i].Method,
			Time:       chain.Epoch(dbRes[i].Epoch).Time(),
			From:       dbRes[i].From,
			To:         dbRes[i].To,
			Amount:     dbRes[i].Amount.Div(decimal.New(1, int32(dbRes[i].Decimal))),
			TokenName:  dbRes[i].TokenName,
			ContractID: dbRes[i].ContractId,
			IconUrl:    mp[dbRes[i].ContractId],
		})
	}
	return &filscan.ERC20AddrTransfersReply{
		Total: total,
		Items: reply,
	}, nil
}

func (e ERC20Biz) ERC20AddrTransfersTokenTypes(ctx context.Context, request *filscan.ERC20AddrTransfersTokenTypesReq) (*filscan.ERC20AddrTransfersTokenTypesReply, error) {
	if len(request.Address) == 0 {
		return nil, nil
	}
	var err error
	addr := strings.ToLower(request.Address)
	if request.Address[0] == 'f' && request.Address[1] == '0' {
		addr, err = ToEthAddr(addr)
		if err != nil {
			return nil, err
		}
	} else if request.Address[0] == 'f' {
		actor, err := e.adapter.Actor(ctx, chain.SmartAddress(request.Address), nil)
		if err != nil {
			log.Errorf("get actor %s info failed, %w", request.Address, err)
			return nil, nil
		}

		if actor.DelegatedAddr != "" && actor.DelegatedAddr != "<empty>" {
			addr, err = ToEthAddr(actor.DelegatedAddr)
		} else {
			addr, err = ToEthAddr(actor.ActorID)
		}
		if err != nil {
			return nil, err
		}
	}
	tokenNames, err := e.repo.GetERC20TransferTokenNamesByRelatedAddr(ctx, addr)
	if err != nil {
		return nil, err
	}
	return &filscan.ERC20AddrTransfersTokenTypesReply{TokenNames: tokenNames}, nil
}

func (e ERC20Biz) ERC20TokenHolder(ctx context.Context, request *filscan.ERC20TokenHolderRequest) (*filscan.ERC20TokenHolderReply, error) {
	balance, err := erc20.GetBalance(e.abi, request.ContractID, request.Address)
	if err != nil {
		return nil, err
	}
	decimal := erc20.GetDecimal(e.abi, request.ContractID)
	return &filscan.ERC20TokenHolderReply{Amount: balance, Decimal: decimal}, nil
}

func ToEthAddr(addr string) (string, error) {
	addrFilecoin, err := address.NewFromString(addr)
	if err != nil {
		return "", err
	}
	ethAddr, err := ethtypes.EthAddressFromFilecoinAddress(addrFilecoin)
	if err != nil {
		return "", err
	}
	return strings.ToLower(ethAddr.String()), nil
}

func (e ERC20Biz) ERC20OwnerTokenList(ctx context.Context, request *filscan.ERC20HolderRequest) (*filscan.ERC20HolderReply, error) {
	if len(request.Address) == 0 {
		return nil, nil
	}
	var err error
	addr := strings.ToLower(request.Address)
	if request.Address[0] == 'f' && request.Address[1] == '0' {
		addr, err = ToEthAddr(addr)
		if err != nil {
			return nil, err
		}
	} else if request.Address[0] == 'f' {
		var actor *londobell.ActorState
		actor, err = e.adapter.Actor(ctx, chain.SmartAddress(request.Address), nil)
		if err != nil {
			log.Errorf("get actor %s info failed, %w", request.Address, err)
			return nil, nil
		}
		addr, err = ToEthAddr(actor.ActorID)
		if err != nil {
			return nil, err
		}
	}

	res, err := e.repo.GetERC20AmountOfOneAddress(ctx, addr)
	if err != nil {
		return nil, err
	}
	reply := filscan.ERC20HolderReply{
		Total:      0,
		Items:      nil,
		TotalValue: decimal.Decimal{},
	}
	value := decimal.Zero
	contractIds := []string{}
	for i := range res {
		item := e.queryErc20(res[i])
		if item == nil {
			continue
		}
		value = value.Add(item.Value)
		reply.Items = append(reply.Items, *item)
		contractIds = append(contractIds, res[i].ContractId)
	}
	icons, err := e.repo.GetContractsUrl(ctx, contractIds)
	if err != nil {
		return nil, fmt.Errorf("get icons failed")
	}
	iconUrlMap := map[string]string{}
	for i := range icons {
		iconUrlMap[icons[i].ContractId] = icons[i].Url
	}
	for i := range reply.Items {
		reply.Items[i].IconUrl = iconUrlMap[reply.Items[i].ContractID]
	}

	reply.Total = len(reply.Items)
	reply.TotalValue = value
	return &reply, nil
}

func (e ERC20Biz) queryErc20(bal *po.FEvmERC20Balance) (item *filscan.ERC20HolderItem) {

	g := parallel.NewGroup()

	var ex _ave.ExchangeInfo
	var tokenName string
	var dec int
	var amount decimal.Decimal

	contract, err := e.repo.GetOneERC20Contract(context.Background(), bal.ContractId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			contract = nil
			err = nil
		} else {
			return
		}
	}
	g.Go(func() error {
		ex = _ave.GetTokenExchangeInfo(bal.ContractId)
		return nil
	})
	g.Go(func() error {
		if contract != nil {
			tokenName = contract.TokenName
		} else {
			tokenName = erc20.GetTokenSymbol(e.abi, bal.ContractId)
		}
		return nil
	})
	g.Go(func() error {
		if contract != nil {
			dec = contract.Decimal
		} else {
			dec = erc20.GetDecimal(e.abi, bal.ContractId)
		}
		return nil
	})
	g.Go(func() error {
		r, er := erc20.GetBalance(e.abi, bal.ContractId, bal.Owner)
		if er != nil {
			return er
		}
		amount = r
		return nil
	})

	err = g.Wait()
	var realAmount decimal.Decimal
	if err != nil {
		realAmount = bal.Amount.Div(decimal.New(1, int32(dec)))
	} else {
		realAmount = amount.Div(decimal.New(1, int32(dec)))
	}
	if realAmount.Equal(decimal.Zero) {
		return
	}
	v := ex.LatestPrice.Mul(realAmount)

	item = &filscan.ERC20HolderItem{
		ContractID: bal.ContractId,
		TokenName:  tokenName,
		Amount:     realAmount,
		Value:      v,
	}
	return
}

func (e ERC20Biz) ERC20DexTrade(ctx context.Context, request *filscan.ERC20TransferReq) (*filscan.ERC20DexTradeListReply, error) {
	total, transfers, err := e.repo.GetERC20SwapInfoByContract(ctx, request.ContractID, int(request.Page), int(request.Limit))
	if err != nil {
		return nil, err
	}

	reply := &filscan.ERC20DexTradeListReply{
		Total: total,
		Items: nil,
	}

	for i := range transfers {
		var swapRate decimal.Decimal
		v1 := transfers[i].AmountIn.Div(decimal.New(1, int32(transfers[i].AmountInDecimal)))
		v2 := transfers[i].AmountOut.Div(decimal.New(1, int32(transfers[i].AmountOutDecimal)))
		swapTokenName := ""
		if TokenNameTrans(transfers[i].AmountInTokenName) == "FIL" {
			swapRate = v1.Div(v2)
			swapTokenName = "FIL"
		} else if TokenNameTrans(transfers[i].AmountOutTokenName) == "FIL" {
			swapRate = v2.Div(v1)
			swapTokenName = "FIL"
		} else if strings.EqualFold(strings.ToLower(transfers[i].AmountInContractId), strings.ToLower(request.ContractID)) {
			swapRate = v1.Div(v2)
			swapTokenName = transfers[i].AmountInTokenName
		} else {
			swapRate = v2.Div(v1)
			swapTokenName = transfers[i].AmountOutTokenName

		}
		dex := e.GetDexInfo(ctx, transfers[i].Dex)
		reply.Items = append(reply.Items, &filscan.ERC20DexInfo{
			Cid:                transfers[i].Cid,
			Action:             transfers[i].Action,
			Time:               chain.Epoch(transfers[i].Epoch).Time(),
			AmountIn:           transfers[i].AmountIn.Div(decimal.New(1, int32(transfers[i].AmountInDecimal))),
			AmountOut:          transfers[i].AmountOut.Div(decimal.New(1, int32(transfers[i].AmountOutDecimal))),
			AmountInTokenName:  TokenNameTrans(transfers[i].AmountInTokenName),
			AmountOutTokenName: TokenNameTrans(transfers[i].AmountOutTokenName),
			Dex:                dex.DexName,
			DexUrl:             dex.DexUrl,
			IconUrl:            dex.IconUrl,
			SwapRate:           &swapRate,
			Value:              &transfers[i].Values,
			SwapTokenName:      swapTokenName,
		})
	}
	return reply, nil
}

func (e ERC20Biz) ERC20Owner(ctx context.Context, request *filscan.ERC20TransferReq) (*filscan.ERC20OwnerListReply, error) {
	contracts, err := e.repo.GetOneERC20Contract(ctx, request.ContractID)
	if err != nil {
		return nil, err
	}
	ex := _ave.GetTokenExchangeInfo(contracts.ContractId)

	total, res, err := e.repo.GetERC20BalanceByContract(ctx, request.ContractID, request.Filter, int(request.Page), int(request.Limit))
	if err != nil {
		log.Error("get erc20 balance by contract failed", err)
		return nil, err
	}

	reply := &filscan.ERC20OwnerListReply{
		Total: total,
		Items: nil,
	}

	for i := range res {
		amout := res[i].Amount.Div(decimal.New(1, int32(contracts.Decimal)))
		rate := amout.Div(contracts.TotalSupply).Mul(decimal.New(1, 2))
		reply.Items = append(reply.Items, &filscan.ERC20Owner{
			Owner:  res[i].Owner,
			Rank:   int((request.Page)*request.Limit) + i + 1,
			Amount: amout,
			Rate:   rate,
			Value:  amout.Mul(ex.LatestPrice),
		})
	}
	return reply, nil
}

func (e ERC20Biz) ERC20Market(ctx context.Context, contractID *filscan.ERC20SummaryRequest) (*filscan.ERC20MarketReply, error) {
	contracts, err := e.repo.GetOneERC20Contract(ctx, contractID.ContractID)
	if err != nil {
		return nil, err
	}
	ex := _ave.GetTokenExchangeInfo(contracts.ContractId)

	latest := ex.LatestPrice

	marketCap := latest.Mul(contracts.TotalSupply)

	return &filscan.ERC20MarketReply{
		ContractID:  contracts.ContractId,
		LatestPrice: latest.String(),
		MarketCap:   marketCap.String(),
		TokenName:   contracts.TokenName,
	}, nil
}

func TokenNameTrans(tokenName string) string {
	if strings.ToLower(tokenName) == "wrapped fil" || strings.ToLower(tokenName) == "wfil" {
		return "FIL"
	}
	return tokenName
}

func DexNameTrans(dexName string) string {
	if v, ok := mpContractIdToDexName[dexName]; ok {
		return v
	}
	return dexName
}

func (e ERC20Biz) GetDexInfo(ctx context.Context, contractId string) *po.DexInfo {
	lk.Lock()
	defer lk.Unlock()
	if v, ok := mpContractIdToDexInfo[contractId]; ok {
		return v
	}
	dexInfo, err := e.repo.GetDexInfo(ctx, contractId)
	if err == nil && dexInfo != nil {
		mpContractIdToDexInfo[contractId] = dexInfo
		return dexInfo
	}

	return &po.DexInfo{}
}

func DexUrlTrans(dexName string) string {
	if v, ok := mpContractIdToDexUrl[dexName]; ok {
		return v
	}
	return ""
}

func (e ERC20Biz) SwapInfoInMessage(ctx context.Context, request *filscan.ERC20TransferInMessageReq) (*filscan.SwapInfoReply, error) {
	dbRes, err := e.repo.GetERC20SwapInfoByCid(ctx, request.Cid)
	if err != nil {
		return nil, err
	}
	if dbRes == nil {
		return nil, nil
	}

	dex := e.GetDexInfo(ctx, dbRes.Dex)
	return &filscan.SwapInfoReply{SwapInfo: &filscan.SwapInfo{
		AmountIn:           dbRes.AmountIn.Div(decimal.New(1, int32(dbRes.AmountInDecimal))),
		AmountOut:          dbRes.AmountOut.Div(decimal.New(1, int32(dbRes.AmountOutDecimal))),
		AmountInTokenName:  TokenNameTrans(dbRes.AmountInTokenName),
		AmountOutTokenName: TokenNameTrans(dbRes.AmountOutTokenName),
		Dex:                dex.DexName,
		DexUrl:             dex.DexUrl,
		IconUrl:            dex.IconUrl,
	}}, nil
}

var mpContractIdToDexName map[string]string
var mpContractIdToDexUrl map[string]string
var mpContractIdToDexInfo map[string]*po.DexInfo
var lk sync.Mutex

func init() {
	mpContractIdToDexInfo = map[string]*po.DexInfo{}
	// mpContractIdToDexName = map[string]string{}
	// mpContractIdToDexUrl = map[string]string{}
	// mpContractIdToDexName["f410fnqu4intkf6nml4ed54zzjwmetmbknegahd75f7a"] = "FileDoge"
	// mpContractIdToDexName["0x6c29c4366a2f9ac5f083ef3394d9849b02a690c0"] = "FileDoge"
	// mpContractIdToDexName["0xcb2d69d97769b7e97f873b53051cd36645f205a3"] = "ThemisPro"
	// mpContractIdToDexName["f410fzmwwtwlxng36s74hhnjqkhgtmzc7ebnde6r5t7a"] = "ThemisPro"
	// mpContractIdToDexName["0x4f180e118e8b20eeb87899ce6a497d05dc8319b6"] = "ThemisPro"
	// mpContractIdToDexName["f410fj4ma4emormqo5odythhgusl5axoiggnwzfx4nqa"] = "ThemisPro"
	//
	// mpContractIdToDexUrl["f410fnqu4intkf6nml4ed54zzjwmetmbknegahd75f7a"] = "https://www.filedoge.io/swap/#/swap"
	// mpContractIdToDexUrl["0x6c29c4366a2f9ac5f083ef3394d9849b02a690c0"] = "https://www.filedoge.io/swap/#/swap"
	// mpContractIdToDexUrl["f410fzmwwtwlxng36s74hhnjqkhgtmzc7ebnde6r5t7a"] = "https://swap.themis.capital/#/swap"
	// mpContractIdToDexUrl["0xcb2d69d97769b7e97f873b53051cd36645f205a3"] = "https://swap.themis.capital/#/swap"
	// mpContractIdToDexUrl["0x4f180e118e8b20eeb87899ce6a497d05dc8319b6"] = "https://swap.themis.capital/#/swap"
	// mpContractIdToDexUrl["f410fj4ma4emormqo5odythhgusl5axoiggnwzfx4nqa"] = "https://swap.themis.capital/#/swap"
}

func (e ERC20Biz) EventsInMessage(ctx context.Context, request *filscan.EventsInMessageReq) (*filscan.EventsInMessageReply, error) {
	res, err := e.agg.GetTransactionReceiptByCid(ctx, request.Cid)
	if err == impl.ErrNotFound || res == nil {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	sigs := []string{}
	sigMaps := map[string]string{}
	for i := range res.Logs {
		if len(res.Logs[i].Topics) > 0 {
			sigs = append(sigs, res.Logs[i].Topics[0])
		}
	}
	dbSigs, err := e.repo.GetEvmEventSignatures(ctx, sigs)
	if err != nil {
		return nil, err
	}

	for i := range dbSigs {
		sigMaps[dbSigs[i].HexSignature] = dbSigs[i].TextSignature
	}
	reply := filscan.EventsInMessageReply{}
	for i := range res.Logs {
		log := res.Logs[i]
		var name *string
		if len(log.Topics) > 0 {
			if v, ok := sigMaps[log.Topics[0]]; ok {
				name = &v
			}
		}
		reply.Logs = append(reply.Logs, filscan.EventsLog{
			Address:  log.Address,
			Name:     name,
			Topics:   log.Topics,
			Data:     log.Data,
			LogIndex: log.LogIndex,
			Removed:  log.Removed,
		})
	}
	return &reply, nil
}

func (e ERC20Biz) ERC20Summary(ctx context.Context, contractID *filscan.ERC20SummaryRequest) (*filscan.ERC20SummaryReply, error) {
	contracts, err := e.repo.GetOneERC20Contract(ctx, contractID.ContractID)
	if err != nil {
		return nil, err
	}

	owners, err := e.repo.GetUniqueTokenHolderByContract(ctx, contractID.ContractID)
	if err != nil {
		return nil, err
	}

	transfers, _, err := e.repo.GetERC20TransferByContract(ctx, contractID.ContractID, 0, 1)
	if err != nil {
		return nil, err
	}

	iconUrl, err := e.repo.GetContractsUrl(ctx, []string{contractID.ContractID})
	if err != nil {
		return nil, err
	}
	if len(iconUrl) == 0 {
		return nil, fmt.Errorf("no matched contract icons")
	}
	res := &filscan.ERC20SummaryReply{
		TotalSupply: contracts.TotalSupply,
		Owners:      owners,
		Transfers:   transfers,
		ContractID:  contracts.ContractId,
		TokenName:   contracts.TokenName,
		TwitterLink: contracts.TwitterLink,
		MainSite:    contracts.MainSite,
		IconUrl:     iconUrl[0].Url,
	}

	return res, nil
}

func (e ERC20Biz) ERC20List(ctx context.Context, _ struct{}) (*filscan.ERC20ContractsReply, error) {
	res, err := e.repo.GetAllERC20Contracts(ctx)
	if err != nil {
		return nil, err
	}

	reply := &filscan.ERC20ContractsReply{}

	for i := range res {
		owners, err := e.repo.GetUniqueTokenHolderByContract(ctx, res[i].ContractId)
		if err != nil {
			return nil, err
		}
		ex := _ave.GetTokenExchangeInfo(res[i].ContractId)

		marketCap := ex.LatestPrice.Mul(res[i].TotalSupply)
		reply.Items = append(reply.Items, &filscan.ERC20Contract{
			TokenName:   res[i].TokenName,
			TotalSupply: res[i].TotalSupply,
			Owners:      owners,
			ContractID:  res[i].ContractId,
			LatestPrice: ex.LatestPrice.String(),
			MarketCap:   marketCap.String(),
			Vol24:       ex.Vol24.String(),
			IconUrl:     res[i].Url,
		})
	}

	sort.Slice(reply.Items, func(i, j int) bool {
		v1, _ := decimal.NewFromString(reply.Items[i].Vol24)
		v2, _ := decimal.NewFromString(reply.Items[j].Vol24)
		return v1.GreaterThan(v2)
	})
	return reply, nil
}

func (e ERC20Biz) ERC20Transfer(ctx context.Context, request *filscan.ERC20TransferReq) (*filscan.ERC20TransferReply, error) {
	total, dbRes, err := e.repo.GetERC20TransferByContract(ctx, request.ContractID, int(request.Page), int(request.Limit))
	if err != nil {
		return nil, err
	}
	reply := []*filscan.ERC20Transfer{}

	for i := range dbRes {
		reply = append(reply, &filscan.ERC20Transfer{
			Cid:       dbRes[i].Cid,
			Method:    dbRes[i].Method,
			Time:      chain.Epoch(dbRes[i].Epoch).Time(),
			From:      dbRes[i].From,
			To:        dbRes[i].To,
			Amount:    dbRes[i].Amount.Div(decimal.New(1, int32(dbRes[i].Decimal))),
			TokenName: dbRes[i].TokenName,
		})
	}
	return &filscan.ERC20TransferReply{
		Total: total,
		Items: reply,
	}, nil
}

func (e ERC20Biz) ERC20TransferInMessage(ctx context.Context, request *filscan.ERC20TransferInMessageReq) (*filscan.ERC20TransferInMessageReply, error) {
	dbRes, err := e.repo.GetERC20TransferInMessage(ctx, request.Cid)
	if err != nil {
		return nil, err
	}

	reply := []*filscan.ERC20Transfer{}

	for i := range dbRes {
		reply = append(reply, &filscan.ERC20Transfer{
			Cid:       dbRes[i].Cid,
			Method:    dbRes[i].Method,
			Time:      chain.Epoch(dbRes[i].Epoch).Time(),
			From:      dbRes[i].From,
			To:        dbRes[i].To,
			Amount:    dbRes[i].Amount.Div(decimal.New(1, int32(dbRes[i].Decimal))),
			TokenName: dbRes[i].TokenName,
		})
	}

	return &filscan.ERC20TransferInMessageReply{
		Items: reply,
	}, nil

}

func (e ERC20Biz) InternalTransfer(ctx context.Context, request *filscan.InternalTransferReq) (*filscan.InternalTransferReply, error) {
	res, err := e.agg.ChildCallsForMessage(ctx, request.Cid)
	if err == impl.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	reply := &filscan.InternalTransferReply{}
	for i := range res[0].InnerCalls {
		value := res[0].InnerCalls[i].Value
		decimals, err := decimal.NewFromString(value)
		if err != nil {
			log.Error("convert decimal failed")
			decimals = decimal.Zero
		}
		reply.InternalTransfers = append(reply.InternalTransfers, filscan.InternalTransfer{
			Method: res[0].InnerCalls[i].MethodName,
			From:   res[0].InnerCalls[i].From.Address(),
			To:     res[0].InnerCalls[i].To.Address(),
			Value:  decimals,
		})
	}

	return reply, nil
}

var _ filscan.ERC20API = (*ERC20Biz)(nil)
