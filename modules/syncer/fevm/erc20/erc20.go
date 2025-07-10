package erc20

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	logging "github.com/gozelle/logger"
	"github.com/hashicorp/go-multierror"
	"github.com/shopspring/decimal"
	cbg "github.com/whyrusleeping/cbor-gen"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/impl"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_ave"
)

var log = logging.NewLogger("erc20")

var _ syncer.Task = (*ERC20Task)(nil)

const (
	SwapTopicHash     = "0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822"
	TransferTopicHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

func (e *ERC20Task) Exec(ctx *syncer.Context) (err error) {

	if ctx.Empty() {
		return
	}

	r, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}

	traces := r.([]*londobell.TraceMessage)

	var multiErr multierror.Error

	// find all subcalls include evm
	idx := map[string]struct{}{}
	for i := range traces {
		if traces[i] == nil {
			continue
		}
		ss := strings.Split(traces[i].Actor, "/")
		if len(ss) != 0 && (ss[len(ss)-1] == "evm" || ss[len(ss)-1] == "eam") {
			ss := strings.Split(traces[i].ID, "-")
			if len(ss) < 2 {
				log.Error("split trace id, len less than 2", traces[i].ID)
				continue
			}
			idx[ss[1]] = struct{}{}
		}
	}

	wg := sync.WaitGroup{}
	errChan := make(chan error, len(traces))
	resBalancesChan := make(chan []*po.FEvmERC20Balance, len(traces))
	resSwapInfoChan := make(chan []*po.FEvmERC20SwapInfo, len(traces))
	resTransfersChan := make(chan []*po.FEvmERC20Transfer, len(traces))
	limitChan := make(chan struct{}, 16)
	for _, v := range traces {
		if v == nil {
			continue
		}

		if v.Depth != 1 {
			continue
		}
		ss := strings.Split(v.ID, "-")
		if len(ss) < 2 {
			log.Error("split trace id, len less than 2", v.ID)
			continue
		}

		if _, ok := idx[ss[1]]; !ok {
			continue
		}
		wg.Add(1)
		v := v
		limitChan <- struct{}{}
		go func() {
			defer func() {
				wg.Done()
				<-limitChan
			}()
			balances, transfers, swapInfos, err := e.handleERC20Transfer(ctx, v)

			if err != nil {
				errChan <- err
				return
			}
			resBalancesChan <- balances
			resTransfersChan <- transfers
			resSwapInfoChan <- swapInfos
		}()
	}
	wg.Wait()
	close(errChan)
	close(resBalancesChan)
	close(resSwapInfoChan)
	close(resTransfersChan)
	for i := range errChan {
		multiErr = *multierror.Append(&multiErr, i)
	}
	if len(multiErr.Errors) != 0 {
		return multiErr.ErrorOrNil()
	}
	balanceRes := []*po.FEvmERC20Balance{}
	for i := range resBalancesChan {
		balanceRes = append(balanceRes, i...)
	}
	balanceRes, err = dal.UniqueERC20BalanceData(balanceRes)
	if err != nil {
		return err
	}

	for i := range balanceRes {
		amount, err := GetBalance(e.abiDecoder, balanceRes[i].ContractId, balanceRes[i].Owner)
		if err != nil {
			return err
		}
		balanceRes[i].Amount = amount
	}

	if len(balanceRes) != 0 {
		err = e.repo.UpsertERC20BalanceBatch(ctx.Context(), balanceRes)
		if err != nil {
			return err
		}
	}

	transferRes := []*po.FEvmERC20Transfer{}
	for i := range resTransfersChan {
		transferRes = append(transferRes, i...)
	}
	err = e.repo.CreateERC20TransferBatch(ctx.Context(), transferRes)
	if err != nil {
		return err
	}

	swapInfoRes := []*po.FEvmERC20SwapInfo{}
	for i := range resSwapInfoChan {
		swapInfoRes = append(swapInfoRes, i...)
	}
	err = e.repo.CreateERC20SwapInfoBatch(ctx.Context(), swapInfoRes)
	if err != nil {
		return err
	}

	// 30 minutes a round下·下··
	if len(traces) != 0 && traces[0].Epoch%60 == 0 && time.Now().Unix()-ctx.Epoch().Unix() < 30*60 {
		log.Info("start fresh erc20 tokens")
		err = e.handleFreshErc20Tokens(ctx.Context())
		multiErr = *multierror.Append(&multiErr, err)
	}
	return multiErr.ErrorOrNil()
}

func (e *ERC20Task) handleFreshErc20Tokens(ctx context.Context) error {
	contracts, err := e.repo.GetAllERC20Contracts(ctx)
	if err != nil {
		return err
	}

	for i := range contracts {
		totalSupply, err := GetTotalSupply(e.abiDecoder, contracts[i].ContractId)
		if err != nil {
			log.Errorf("error in get total supply %w", err)
			continue
		}
		decimals := GetDecimal(e.abiDecoder, contracts[i].ContractId)
		totalSupply = totalSupply.Div(decimal.New(1, int32(decimals)))
		totalSupply = totalSupply.Ceil()
		if contracts[i].TotalSupply == totalSupply {
			continue
		}
		contracts[i].TotalSupply = totalSupply
		err = e.repo.UpdateOneERC20Contract(ctx, contracts[i].ContractId, contracts[i])
		if err != nil {
			log.Errorf("error in get update one erc20 contract", err)
			continue
		}
	}

	list, err := e.repo.GetAllERC20FreshContracts(ctx)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	for i := range list {
		wg.Add(1)
		ls := list[i]
		go func() {
			defer wg.Done()
			max, err := e.repo.GetUniqueTokenHolderByContract(ctx, ls.ContractId)
			if err != nil {
				log.Errorf("get unique token holder failed: %w", err)
				return
			}
			_, owners, err := e.repo.GetERC20BalanceByContract(ctx, ls.ContractId, "", 0, int(max))
			if err != nil {
				log.Errorf("get erc20 balance by contract failed: %w", err)
				return
			}
			batchUpdate := []*po.FEvmERC20Balance{}
			for j := range owners {
				d, err := GetBalance(e.abiDecoder, owners[j].ContractId, owners[j].Owner)
				if err != nil {
					continue
				}
				if d != owners[j].Amount {
					batchUpdate = append(batchUpdate, &po.FEvmERC20Balance{
						Owner:      owners[j].Owner,
						ContractId: owners[j].ContractId,
						Amount:     d,
					})
				}
			}
			err = e.repo.UpsertERC20BalanceBatch(ctx, batchUpdate)
			if err != nil {
				log.Error("upsert erc20 balance batch", err)
				return
			}
		}()
	}
	wg.Wait()
	return nil
}

func GetDecimal(abiDecoder filscan.ABIDecoderAPI, contractID string) int {
	tokenName := contractDecimalCache.getTokenDecimal(contractID)
	if tokenName != nil {
		return *tokenName
	}

	r, err := abiDecoder.CallContract([]byte(ERC20ABI), contractID, "decimals", nil)
	if err != nil {
		log.Error("Call err", err)
		return 1
	}

	if len(r) != 1 {
		log.Error("result len not match")
		return 1
	}

	if v, ok := r[0].(float64); ok {
		contractDecimalCache.setTokenDecimal(contractID, int(v))
		return int(v)
	}
	log.Error("type assert failed")

	return 1
}

func GetTotalSupply(abiDecoder filscan.ABIDecoderAPI, contractID string) (decimal.Decimal, error) {
	r, err := abiDecoder.CallContract([]byte(ERC20ABI), contractID, "totalSupply", nil)
	if err != nil {
		log.Error("Call err", err)
		return decimal.Decimal{}, err
	}

	if len(r) != 1 {
		log.Error("result len not match")
		return decimal.Decimal{}, err
	}

	if v, ok := r[0].(float64); ok {
		return decimal.NewFromFloat(v), nil
	}
	log.Error("type assert failed")

	return decimal.Decimal{}, nil
}

func GetBalance(abiDecoder filscan.ABIDecoderAPI, contractID, addr string) (decimal.Decimal, error) {
	r, err := abiDecoder.CallContract([]byte(ERC20ABI), contractID, "balanceOf",
		[]*filscan.ContractParam{{
			Type:  "address",
			Value: addr,
		}})
	if err != nil {
		log.Error("Call err", err)
		return decimal.Decimal{}, err
	}

	if len(r) != 1 {
		log.Error("result len not match")
		return decimal.Decimal{}, fmt.Errorf("result len not match")
	}

	if v, ok := r[0].(float64); ok {
		return decimal.NewFromFloat(v), nil
	}
	log.Error("type assert failed")

	return decimal.Decimal{}, fmt.Errorf("type assert failed")
}

func (e *ERC20Task) GetTokenName(contractID string) string {
	tokenName := contractNameCache.getTokenName(contractID)
	if tokenName != nil {
		return *tokenName
	}

	r, err := e.abiDecoder.CallContract([]byte(ERC20ABI), contractID, "name", nil)
	if err != nil {
		log.Error("Call err", err)
		return ""
	}

	if len(r) != 1 {
		log.Error("result len not match")
		return ""
	}
	if v, ok := r[0].(string); ok {
		contractNameCache.setTokenName(contractID, v)
		return v
	} else {
		log.Error("r0 type assert failed")
	}
	return ""
}

func tryGetTokenSymbol(abiDecoder filscan.ABIDecoderAPI, contractID string) error {
	tokenName := contractTokenSymbolCache.getTokenName(contractID)
	if tokenName != nil {
		return nil
	}

	r, err := abiDecoder.CallContract([]byte(ERC20ABI), contractID, "symbol", nil)
	if err != nil {
		log.Error("TryGetTokenSymbol filter call err", err)
		return err
	}

	if len(r) != 1 {
		log.Error("TryGetTokenSymbol filter result len not match")
		return fmt.Errorf("TryGetTokenSymbol filter result len not match")
	}
	if v, ok := r[0].(string); ok {
		contractTokenSymbolCache.setTokenName(contractID, v)
		return nil
	} else {
		log.Error("TryGetTokenSymbol filter r0 type assert failed")
		return fmt.Errorf("TryGetTokenSymbol filter r0 type assert failed")
	}
}

func GetTokenSymbol(abiDecoder filscan.ABIDecoderAPI, contractID string) string {
	tokenName := contractTokenSymbolCache.getTokenName(contractID)
	if tokenName != nil {
		return *tokenName
	}

	r, err := abiDecoder.CallContract([]byte(ERC20ABI), contractID, "symbol", nil)
	if err != nil {
		log.Error("Call err", err)
		return ""
	}

	if len(r) != 1 {
		log.Error("result len not match")
		return ""
	}
	if v, ok := r[0].(string); ok {
		contractTokenSymbolCache.setTokenName(contractID, v)
		return v
	} else {
		log.Error("r0 type assert failed")
	}
	return ""
}

func (e *ERC20Task) IsPair(contractID string) (bool, error) {
	isPair := contractIsPairCache.getTokenName(contractID)
	if isPair != nil {
		if *isPair == "true" {
			return true, nil
		}
		return false, nil
	}
	ethAddr, err := ethtypes.ParseEthAddress(contractID)
	if err != nil {
		return false, err
	}
	predefinedBlock := "latest"
	ebytes, err := e.node.EthGetCode(context.TODO(), ethAddr, ethtypes.EthBlockNumberOrHash{
		PredefinedBlock: &predefinedBlock,
	})
	if err != nil {
		return false, err
	}
	res, err := e.abiDecoder.DetectContractProtocol(ebytes)
	if err != nil {
		return false, err
	}
	if res.Pair {
		contractIsPairCache.setTokenName(contractID, "true")
	} else {
		contractIsPairCache.setTokenName(contractID, "false")
	}
	return res.Pair, err
}

func (e *ERC20Task) handleERC20Transfer(ctx *syncer.Context, trace *londobell.TraceMessage) ([]*po.FEvmERC20Balance, []*po.FEvmERC20Transfer, []*po.FEvmERC20SwapInfo, error) {
	cids := trace.Cid
	if trace.SignedCid != nil && *trace.SignedCid != "" {
		cids = *trace.SignedCid
	}

	receipt, err := ctx.Agg().GetTransactionReceiptByCid(ctx.Context(), cids)
	if err == impl.ErrNotFound {
		log.Error(err)
		return nil, nil, nil, nil
	}
	if err != nil {
		log.Error(err)
		return nil, nil, nil, fmt.Errorf("get transaction receipt by cid failed: %w", err)
	}
	if receipt == nil || 0 == len(receipt.Logs) {
		return nil, nil, nil, nil
	}
	items := []*po.FEvmERC20Transfer{}
	items2 := []*po.FEvmERC20Balance{}
	// TODO: bind abi and use abiDecoder to get a serialize name

	method := ""
	if trace.ParamsBson != nil && len(trace.ParamsBson.Data) != 0 {
		buffer := bytes.NewBuffer(trace.ParamsBson.Data)
		paramsByte, err := cbg.ReadByteArray(buffer, uint64(len(trace.ParamsBson.Data)))
		if err == nil {
			method = "0x" + hex.EncodeToString(paramsByte)
		} else {
			log.Warn(err)
		}
	}

	if len(method) > 10 {
		method = method[:10]
		e.mapReadLock.RLock()
		ms := e.methodMap[method]
		e.mapReadLock.RUnlock()
		if ms != "" {
			method = ms
		}
	}

	cnt := 0

	for i := range receipt.Logs {
		// 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef -> Transfer
		if len(receipt.Logs[i].Topics) != 3 || receipt.Logs[i].Topics[0] != TransferTopicHash {
			continue
		}
		if tryGetTokenSymbol(e.abiDecoder, receipt.Logs[i].Address) != nil {
			continue
		}
		bigInt := new(big.Int)
		amount, ok := bigInt.SetString(receipt.Logs[i].Data[2:], 16)
		if !ok {
			err = fmt.Errorf("translate receipt data failed %v", receipt.Logs[i].Data[2:])
			log.Error(err)
			continue
		}

		amountDecimal, err := decimal.NewFromString(amount.String())
		if err != nil {
			log.Errorf("parse amount into decimal failed: %w", err)
			continue
		}
		from := receipt.Logs[i].Topics[1][:2] + receipt.Logs[i].Topics[1][26:]
		to := receipt.Logs[i].Topics[2][:2] + receipt.Logs[i].Topics[2][26:]
		contractAddr := receipt.Logs[i].Address
		//amount1, err := GetBalance(o.abiDecoder, contractAddr, from)
		//if err != nil {
		//	return err
		//}
		items2 = append(items2, &po.FEvmERC20Balance{
			Owner:      from,
			ContractId: contractAddr,
			//Amount:     amount1,
		})

		//amount2, err := GetBalance(o.abiDecoder, contractAddr, to)
		//if err != nil {
		//	return err
		//}
		items2 = append(items2, &po.FEvmERC20Balance{
			Owner:      to,
			ContractId: contractAddr,
			//Amount:     amount2,
		})

		items = append(items, &po.FEvmERC20Transfer{
			Epoch:      trace.Epoch,
			Cid:        cids,
			ContractId: receipt.Logs[i].Address,
			// get correct address
			From:      from,
			To:        to,
			Amount:    amountDecimal,
			DEX:       receipt.To,
			TokenName: GetTokenSymbol(e.abiDecoder, receipt.Logs[i].Address),
			Method:    method,
			Decimal:   GetDecimal(e.abiDecoder, receipt.Logs[i].Address),
			Index:     cnt,
		})
		cnt++
	}

	isSwap := false
	for i := range receipt.Logs {
		if receipt.Logs[i].Topics[0] == SwapTopicHash {
			isSwap = true
			break
		}
	}

	res2 := []*po.FEvmERC20SwapInfo{}
	if isSwap {
		var firstLog, endLog *londobell.EthReceiptLog
		//mp := map[string]int{}
		for i := range receipt.Logs {
			if receipt.Logs[i].Topics[0] != TransferTopicHash {
				continue
			}
			from := receipt.Logs[i].Topics[1][:2] + receipt.Logs[i].Topics[1][26:]
			to := receipt.Logs[i].Topics[2][:2] + receipt.Logs[i].Topics[2][26:]
			isPair, err := e.IsPair(from)
			if err != nil {
				return nil, nil, nil, err
			}
			if isPair {
				endLog = receipt.Logs[i]
			}
			isPair, err = e.IsPair(to)
			if err != nil {
				return nil, nil, nil, err
			}
			if isPair && firstLog == nil {
				firstLog = receipt.Logs[i]
			}
			//if firstLog == nil && {
			//
			//}
			//if v, ok := mp[fmt.Sprintf("%s-%s", receipt.Logs[i].Topics[2], receipt.Logs[i].Topics[1])]; ok {
			//	firstLog = receipt.Logs[v]
			//	endLog = receipt.Logs[i]
			//	break
			//}
			//mp[fmt.Sprintf("%s-%s", receipt.Logs[i].Topics[1], receipt.Logs[i].Topics[2])] = i

			//if firstLog == nil {
			//	firstLog = receipt.Logs[i]
			//}
			//endLog = receipt.Logs[i]
		}
		if firstLog == nil || endLog == nil {
			log.Errorf("parse swap log failed hash: %s cid: %s", receipt.TransactionHash, cids)
			goto Return
		}
		bigInt := new(big.Int)
		amount, ok := bigInt.SetString(firstLog.Data[2:], 16)
		if !ok {
			log.Error(err)
			goto Return
		}

		amountDecimal, err := decimal.NewFromString(amount.String())
		if err != nil {
			log.Errorf("parse amount into decimal failed: %w", err)
			goto Return
		}

		bigInt2 := new(big.Int)
		amount2, ok := bigInt2.SetString(endLog.Data[2:], 16)
		if !ok {
			log.Error(err)
			goto Return
		}

		amountDecimal2, err := decimal.NewFromString(amount2.String())
		if err != nil {
			log.Errorf("parse amount into decimal failed: %w", err)
			goto Return
		}

		var value decimal.Decimal
		ex := _ave.GetTokenExchangeInfo(firstLog.Address)
		if !ex.LatestPrice.Equal(decimal.Zero) {
			amounts := amountDecimal.Div(decimal.New(1, int32(GetDecimal(e.abiDecoder, firstLog.Address))))
			value = amounts.Mul(ex.LatestPrice)
		} else {
			ex = _ave.GetTokenExchangeInfo(endLog.Address)
			amounts := amountDecimal2.Div(decimal.New(1, int32(GetDecimal(e.abiDecoder, endLog.Address))))
			value = amounts.Mul(ex.LatestPrice)
		}
		res2 = append(res2, &po.FEvmERC20SwapInfo{
			Cid:                 cids,
			Action:              e.ParseAction(firstLog, endLog),
			Epoch:               int(trace.Epoch),
			AmountIn:            amountDecimal2,
			AmountOut:           amountDecimal,
			AmountInTokenName:   GetTokenSymbol(e.abiDecoder, endLog.Address),
			AmountOutTokenName:  GetTokenSymbol(e.abiDecoder, firstLog.Address),
			Dex:                 receipt.To,
			AmountInContractId:  endLog.Address,
			AmountOutContractId: firstLog.Address,
			AmountInDecimal:     GetDecimal(e.abiDecoder, endLog.Address),
			AmountOutDecimal:    GetDecimal(e.abiDecoder, firstLog.Address),
			SwapRate:            decimal.Decimal{},
			Values:              value,
		})
	}

Return:
	return items2, items, res2, nil
}

// todo: duplicate func
func TokenNameTrans(tokenName string) string {
	if strings.ToLower(tokenName) == "wrapped fil" || strings.ToLower(tokenName) == "wfil" {
		return "fil"
	}
	return tokenName
}

func (e *ERC20Task) ParseAction(first, end *londobell.EthReceiptLog) string {
	if TokenNameTrans(GetTokenSymbol(e.abiDecoder, first.Address)) == "fil" {
		return "buy"
	} else if TokenNameTrans(GetTokenSymbol(e.abiDecoder, end.Address)) == "fil" {
		return "sell"
	}
	return "swap"
}

type ERC20Task struct {
	node        *lotus_api.Node
	repo        repository.ERC20TokenRepo
	methodMap   map[string]string
	mapReadLock sync.RWMutex
	abiDecoder  filscan.ABIDecoderAPI
}

func (e *ERC20Task) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	return nil
}

func (e *ERC20Task) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	fts, err := e.repo.GetERC20TransferBatchAfterEpoch(ctx, int(gteEpoch))
	if err != nil {
		return err
	}
	items := []*po.FEvmERC20Balance{}
	for i := range fts {
		amount1, err := GetBalance(e.abiDecoder, fts[i].ContractId, fts[i].From)
		if err != nil {
			return err
		}
		items = append(items, &po.FEvmERC20Balance{
			Owner:      fts[i].From,
			ContractId: fts[i].ContractId,
			Amount:     amount1,
		})

		amount2, err := GetBalance(e.abiDecoder, fts[i].ContractId, fts[i].To)
		if err != nil {
			return err
		}
		items = append(items, &po.FEvmERC20Balance{
			Owner:      fts[i].To,
			ContractId: fts[i].ContractId,
			Amount:     amount2,
		})
	}

	err = e.repo.UpsertERC20BalanceBatch(ctx, items)
	if err != nil {
		return err
	}
	err = e.repo.CleanERC20SwapInfo(ctx, int(gteEpoch))
	if err != nil {
		return err
	}

	return e.repo.CleanERC20TransferBatch(ctx, int(gteEpoch))
}

func NewERC20Task(abiDecoder filscan.ABIDecoderAPI, repo repository.ERC20TokenRepo, node *lotus_api.Node) *ERC20Task {
	res, err := repo.GetAllMethodsDecodeSignature(context.TODO())
	if err != nil {
		panic(err)
	}

	s := map[string]string{}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Id < res[j].Id
	})
	for i := range res {
		if _, ok := s[res[i].HexSignature]; !ok {
			s[res[i].HexSignature] = res[i].Decode
		}
	}

	return &ERC20Task{
		node:        node,
		repo:        repo,
		abiDecoder:  abiDecoder,
		methodMap:   s,
		mapReadLock: sync.RWMutex{},
	}
}

func (e *ERC20Task) Name() string {
	return "erc20-task"
}
