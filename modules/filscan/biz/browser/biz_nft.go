package browser

import (
	"context"
	"sort"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns/providers"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/nft"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

func NewNFTBiz(db *gorm.DB) *NFTBiz {
	return &NFTBiz{repo: dal.NewNFTQueryer(db), nftUrlDal: nft.NewUrlDal(db)}
}

var _ filscan.NFTAPI = (*NFTBiz)(nil)

type NFTBiz struct {
	repo      repository.NFTQueryer
	nftUrlDal *nft.UrlDal
}

func (n NFTBiz) isFns(contract string) bool {
	switch providers.ToAlias(contract) {
	case providers.Filfox, providers.Opengate:
		return true
	}
	return false
}

func (n NFTBiz) NFTOwners(ctx context.Context, request filscan.NFTOwnersRequest) (reply *filscan.NFTOwnersReply, err error) {

	if n.isFns(request.Contract) {
		return n.fnsOwners(ctx, request)
	}

	reply = &filscan.NFTOwnersReply{}
	items, total, err := n.repo.GetNFTOwners(ctx, request.Contract, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	})
	if err != nil {
		return
	}

	reply.Total = total

	for _, v := range items {
		reply.Items = append(reply.Items, &filscan.NFTOwner{
			Rank:       v.Rank,
			Owner:      v.Owner,
			Amount:     v.Tokens,
			Percentage: v.Percent,
		})
	}
	return
}

func (n NFTBiz) NFTTransfers(ctx context.Context, request *filscan.NFTTransfersRequest) (reply *filscan.NFTTransfersReply, err error) {

	if n.isFns(request.Contract) {
		return n.fnsTransfers(ctx, request)
	}

	reply = &filscan.NFTTransfersReply{}
	items, total, err := n.repo.GetNFTTransfers(ctx, request.Contract, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	})
	if err != nil {
		return
	}

	reply.Total = total

	for _, v := range items {
		reply.Items = append(reply.Items, &filscan.NFTTransfer{
			Cid:    v.Cid,
			Method: v.Method,
			Time:   chain.Epoch(v.Epoch).Time().Unix(),
			From:   v.From,
			To:     v.To,
			Item:   v.Item,
			Value:  v.Value,
			Url:    nft.ResolveNFTURL(ctx, false, n.nftUrlDal, v.Contract, v.TokenId, v.Item, v.TokenUri, v.TokenUrl),
		})
	}
	return
}

func (n NFTBiz) fnsSummary(ctx context.Context, provider string) (reply *filscan.NFTSummaryReply, err error) {

	reply = &filscan.NFTSummaryReply{
		TotalSupply: 0,
		Owners:      0,
		Transfers:   0,
		Contract:    "",
	}

	var p providers.FNSProvider
	switch providers.ToAlias(provider) {
	case providers.Filfox:
		p = providers.GetProvider(providers.Filfox)
	case providers.Opengate:
		p = providers.GetProvider(providers.Opengate)
	}

	reply.Contract = p.Contract
	reply.TokenName = p.Name
	reply.Logo = p.LOGO
	reply.MainSite = p.MainSite
	reply.TwitterLink = p.TwitterLink

	summary, err := n.repo.GetFnsSummary(ctx, providers.ToAlias(provider))
	if err != nil {
		return
	}

	reply.Owners = summary.Controllers
	reply.TotalSupply = summary.Tokens
	reply.Transfers = summary.Transfers

	return
}

type NFTInfo struct {
	Website string
	Twitter string
}

var infos = map[string]NFTInfo{
	"0xa02cbf1dc75058cc6f2f5b8e7a9087425f5248e3": {Website: "https://fil.opengatenft.com/?code=ez0ik3#/list?collection=213", Twitter: ""},
	"0xf7ceaa5da7305b87361f9db6a300bd6d74c674d2": {Website: "https://www.filpunks.io/", Twitter: "https://twitter.com/filpunks314?s=20"},
	"0xbc3a4453dd52d3820eab1498c4673c694c5c6f09": {Website: "https://www.filebunnies.xyz/", Twitter: "https://twitter.com/web3jedis"},
}

func (n NFTBiz) NFTSummary(ctx context.Context, request filscan.NFTSummaryRequest) (reply *filscan.NFTSummaryReply, err error) {

	if n.isFns(request.Contract) {
		return n.fnsSummary(ctx, request.Contract)
	}

	item, err := n.repo.GetNFTSummary(ctx, request.Contract)
	if err != nil {
		return
	}
	reply = &filscan.NFTSummaryReply{
		TotalSupply: item.Mints,
		Owners:      item.Owners,
		Transfers:   item.Transfers,
		Contract:    item.Contract,
		TokenName:   item.Collection,
		Logo:        item.Logo,
		MainSite:    infos[request.Contract].Website,
		TwitterLink: infos[request.Contract].Twitter,
	}

	return
}

func (n NFTBiz) NFTTokens(ctx context.Context, request filscan.NFTTokensRequest) (reply *filscan.NFTTokensReply, err error) {

	filfoxSummary, err := n.repo.GetFnsSummary(ctx, providers.Filfox)
	if err != nil {
		return
	}
	opengateSummary, err := n.repo.GetFnsSummary(ctx, providers.Opengate)
	if err != nil {
		return
	}

	filfoxProvider := providers.GetProvider(providers.Filfox)
	opengateProvider := providers.GetProvider(providers.Opengate)

	reply = &filscan.NFTTokensReply{
		Total: 2,
		Items: []*filscan.NFTToken{},
	}

	if request.Index == 0 {
		reply.Items = append(reply.Items, []*filscan.NFTToken{
			{
				Icon:          filfoxProvider.LOGO,
				Collection:    filfoxProvider.Name,
				TradingVolume: decimal.NewFromFloat(0),
				Holders:       filfoxSummary.Controllers,
				Transfers:     filfoxSummary.Transfers,
				Provider:      providers.ToContract(providers.Filfox),
				Contract:      providers.ToContract(providers.Filfox),
			},
			{
				Icon:          opengateProvider.LOGO,
				Collection:    opengateProvider.Name,
				TradingVolume: decimal.NewFromInt(0),
				Holders:       opengateSummary.Controllers,
				Transfers:     opengateSummary.Transfers,
				Provider:      providers.ToContract(providers.Opengate),
				Contract:      providers.ToContract(providers.Opengate),
			},
		}...)
	}

	tokens, total, err := n.repo.GetNFTSummaries(ctx, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	})
	if err != nil {
		return
	}

	reply.Total += total
	for _, v := range tokens {
		if v.Collection == "" {
			v.Collection = "--"
		}
		reply.Items = append(reply.Items, &filscan.NFTToken{
			Icon:          v.Logo,
			Collection:    v.Collection,
			TradingVolume: decimal.Decimal{},
			Holders:       v.Owners,
			Transfers:     v.Transfers,
			Mints:         v.Mints,
			Provider:      v.Contract,
			Contract:      v.Contract,
		})
	}
	sort.Slice(reply.Items, func(i, j int) bool {
		return reply.Items[i].Holders > reply.Items[j].Holders
	})
	return
}

func (n NFTBiz) NFTMessageTransfers(ctx context.Context, request filscan.NFTMessageTransfersRequest) (reply *filscan.NFTMessageTransfersReply, err error) {

	reply = &filscan.NFTMessageTransfersReply{}

	fnsTransfers, err := n.repo.GetFnsTransfersByCid(ctx, request.Cid)
	if err != nil {
		return
	}

	for _, v := range fnsTransfers {
		reply.Items = append(reply.Items, &filscan.NFTMessageTransfersReplyItem{
			Cid:       v.Cid,
			Method:    v.Method,
			Time:      chain.Epoch(v.Epoch).Time().Unix(),
			From:      chain.TrimHexAddress(v.From),
			To:        chain.TrimHexAddress(v.To),
			Amount:    decimal.NewFromInt(1),
			TokenName: v.Item,
			Provider:  providers.ToContract(v.Provider),
			Contract:  providers.ToContract(v.Provider),
			Item:      v.Item,
			Url:       "",
		})
	}

	nftTransfers, err := n.repo.GetNFTTransfersByCid(ctx, request.Cid)
	if err != nil {
		return
	}

	for _, v := range nftTransfers {
		reply.Items = append(reply.Items, &filscan.NFTMessageTransfersReplyItem{
			Cid:       v.Cid,
			Method:    v.Method,
			Time:      chain.Epoch(v.Epoch).Time().Unix(),
			From:      chain.TrimHexAddress(v.From),
			To:        chain.TrimHexAddress(v.To),
			Amount:    decimal.NewFromInt(1),
			TokenName: v.Item,
			Provider:  v.Contract,
			Contract:  v.Contract,
			Item:      v.Item,
			Url:       nft.ResolveNFTURL(ctx, false, n.nftUrlDal, v.Contract, v.TokenId, v.Item, v.TokenUri, v.TokenUrl),
		})
	}

	return
}

func (n NFTBiz) fnsTransfers(ctx context.Context, request *filscan.NFTTransfersRequest) (reply *filscan.NFTTransfersReply, err error) {

	reply = &filscan.NFTTransfersReply{}

	items, total, err := n.repo.GetFnsTransfers(ctx, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	}, providers.ToAlias(request.Contract))
	if err != nil {
		return
	}

	reply.Total = total

	for _, v := range items {
		reply.Items = append(reply.Items, &filscan.NFTTransfer{
			Cid:    v.Cid,
			Method: v.Method,
			Time:   chain.CalcTimeByEpoch(v.Epoch).Time().Unix(),
			From:   chain.TrimHexAddress(v.From),
			To:     chain.TrimHexAddress(v.To),
			Item:   v.Item,
		})
	}

	return
}

func (n NFTBiz) fnsOwners(ctx context.Context, request filscan.NFTOwnersRequest) (reply *filscan.NFTOwnersReply, err error) {
	reply = &filscan.NFTOwnersReply{}
	items, total, err := n.repo.GetFnsRegistrants(ctx, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	}, providers.ToAlias(request.Contract))
	if err != nil {
		return
	}

	reply.Total = total

	for _, v := range items {
		reply.Items = append(reply.Items, &filscan.NFTOwner{
			Rank:       v.Rank,
			Owner:      v.Registrant,
			Amount:     v.Tokens,
			Percentage: v.Percent,
		})
	}
	return
}
