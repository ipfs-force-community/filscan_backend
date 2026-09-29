package nft

import (
	"context"
	"fmt"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

func NewNFTCalculator(db *gorm.DB, decoder fevm.ABIDecoderAPI) *NFTCalculator {
	return &NFTCalculator{
		mapper:  NewMapper(db),
		decoder: decoder,
		abi:     initErc721MetadataAbi(),
		urlDal:  NewUrlDal(db),
	}
}

var _ syncer.Calculator = (*NFTCalculator)(nil)

type NFTCalculator struct {
	mapper  iMapper
	decoder fevm.ABIDecoderAPI
	urlDal  *UrlDal
	abi     []byte
}

func (e NFTCalculator) Name() string {
	return "nft-calculator"
}

func (e NFTCalculator) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	
	transfers, err := e.mapper.GetTransfersAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}
	
	type TransferToken struct {
		Transfer *po.NFTTransfer
		Token    *po.NFTToken
	}
	
	filter := map[string]*TransferToken{}
	contractMap := map[string]struct{}{}
	
	for _, v := range transfers {
		contractMap[v.Contract] = struct{}{}
		key := fmt.Sprintf("%s.%s", v.Contract, v.TokenId)
		if err != nil {
			return
		}
		if _, ok := filter[key]; ok {
			continue
		}
		filter[key] = &TransferToken{Transfer: v}
	}
	
	var tokens []*po.NFTToken
	for _, v := range filter {
		var token *po.NFTToken
		token, err = prepareErc721Token(e.decoder, e.abi, v.Transfer.Contract, v.Transfer.TokenId)
		if err != nil {
			return
		}
		if token != nil {
			tokens = append(tokens, token)
		} else {
			err = e.mapper.DeleteToken(ctx, v.Transfer.Contract, v.Transfer.TokenId)
			if err != nil {
				return
			}
		}
	}
	
	err = e.mapper.SaveTokens(ctx, tokens)
	if err != nil {
		return
	}
	
	err = e.refreshContracts(ctx, contractMap)
	if err != nil {
		return
	}
	
	return
}

func (e NFTCalculator) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (e NFTCalculator) Calc(ctx *syncer.Context) (err error) {
	if !ctx.LastCalc() {
		return
	}
	
	transfers, err := e.mapper.GetTransfers(ctx.Context(), ctx.Epochs())
	if err != nil {
		return
	}
	
	if len(transfers) == 0 {
		return
	}
	
	filter := map[string]*po.NFTTransfer{}
	contractMap := map[string]struct{}{}
	
	for _, v := range transfers {
		contractMap[v.Contract] = struct{}{}
		key := fmt.Sprintf("%s.%s", v.Contract, v.TokenId)
		if err != nil {
			return
		}
		if _, ok := filter[key]; ok {
			continue
		}
		filter[key] = v
	}
	
	var tokens []*po.NFTToken
	
	tokens, err = e.prepareTokens(filter)
	if err != nil {
		return
	}
	
	var contracts []*po.NFTContract
	for k := range contractMap {
		contracts = append(contracts, &po.NFTContract{Contract: k})
	}
	
	err = e.save(ctx.Context(), tokens, contracts)
	if err != nil {
		return
	}
	
	err = e.refreshContracts(ctx.Context(), contractMap)
	if err != nil {
		return
	}
	
	for _, v := range tokens {
		ResolveNFTURL(ctx.Context(), true, e.urlDal, v.Contract, v.TokenId, v.Item, v.TokenUri, "")
	}
	
	return
}

// prepareTokens 为一批转账事件准备待落库的 NFT token 记录。
//
// 抽出来只为可测（循环体与原来逐字一致，只多了一段 nil 判断）：原实现把
// prepareErc721Token 的返回值直接 append，而该函数对「不是可解析 ERC721 的合约」
// 会返回 (nil, nil)（见 task.go collectTransfers 的跳过语义）——nil 一旦进了 tokens，
// 后面的 SaveTokens（逐字段解引用）与 ResolveNFTURL 都会 nil 解引用 panic，
// 让该高度计算任务失败并被框架无限重试、卡死。RollBack 里一直有同样的 nil 判断，
// Calc 这条路径漏了。跳过是正确的，但要留一条 Warn 便于事后核对丢了哪些 token。
func (e NFTCalculator) prepareTokens(filter map[string]*po.NFTTransfer) (tokens []*po.NFTToken, err error) {
	for _, v := range filter {
		var token *po.NFTToken
		token, err = prepareErc721Token(e.decoder, e.abi, v.Contract, v.TokenId)
		if err != nil {
			return
		}
		if token == nil {
			log.Warnf("epoch %d 跳过 NFT token 落库：合约取不到 ERC721 元数据（token 准备为空，非可解析 ERC721）: contract=%s token_id=%s cid=%s",
				v.Epoch, v.Contract, v.TokenId, v.Cid)
			continue
		}
		tokens = append(tokens, token)
	}
	return
}

func (e NFTCalculator) save(ctx context.Context, tokens []*po.NFTToken, contracts []*po.NFTContract) (err error) {
	err = e.mapper.SaveTokens(ctx, tokens)
	if err != nil {
		return
	}
	
	for _, v := range contracts {
		v.Collection, err = e.mapper.GetContractCollection(ctx, v.Contract)
		if err != nil {
			return
		}
	}
	
	err = e.mapper.SaveContracts(ctx, contracts)
	if err != nil {
		return
	}
	return
}

func (e NFTCalculator) refreshContract(ctx context.Context, contract string) (err error) {
	
	mints, err := e.mapper.CountContractMints(ctx, contract)
	if err != nil {
		return
	}
	
	owners, err := e.mapper.CountContractOwners(ctx, contract)
	if err != nil {
		return
	}
	
	transfers, err := e.mapper.CountContractTransfers(ctx, contract)
	if err != nil {
		return
	}
	
	err = e.mapper.UpdateContractCounts(ctx, contract, mints, owners, transfers)
	if err != nil {
		return
	}
	
	return
}

func (e NFTCalculator) refreshContracts(ctx context.Context, contracts map[string]struct{}) (err error) {
	
	for k := range contracts {
		err = e.refreshContract(ctx, k)
		if err != nil {
			return
		}
	}
	
	return
}
