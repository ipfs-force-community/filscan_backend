package browser

import (
	"context"
	"fmt"
	"github.com/gozelle/async/parallel"
	calc_fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-fns-task"
	
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns/providers"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

func NewFnsBiz(db *gorm.DB, decoder fevm.ABIDecoderAPI) *FnsBiz {
	return &FnsBiz{
		repo:    dal.NewNFTQueryer(db),
		decoder: decoder,
		task:    calc_fns_task.NewCalcFnsTask(decoder, dal.NewFnsSaverDal(db)),
	}
}

var _ filscan.FNSAPI = (*FnsBiz)(nil)

type FnsBiz struct {
	repo    repository.NFTQueryer
	decoder fevm.ABIDecoderAPI
	task    *calc_fns_task.CalcFnsTask
}

func (f FnsBiz) FnsSummary(ctx context.Context, request filscan.FNSFnsSummaryRequest) (reply *filscan.FnsSummaryReply, err error) {
	
	reply = &filscan.FnsSummaryReply{
		TotalSupply: 0,
		Owners:      0,
		Transfers:   0,
		Contract:    "",
	}
	
	var p providers.FNSProvider
	switch providers.ToAlias(request.Provider) {
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
	
	summary, err := f.repo.GetFnsSummary(ctx, providers.ToAlias(request.Provider))
	if err != nil {
		return
	}
	
	reply.Owners = summary.Controllers
	reply.TotalSupply = summary.Tokens
	reply.Transfers = summary.Transfers
	//reply.Provider = request.Provider
	
	return
}

func (f FnsBiz) FnsTransfers(ctx context.Context, request *filscan.FnsTransfersRequest) (reply *filscan.FnsTransfersReply, err error) {
	
	reply = &filscan.FnsTransfersReply{}
	
	items, total, err := f.repo.GetFnsTransfers(ctx, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	}, providers.ToAlias(request.Provider))
	if err != nil {
		return
	}
	
	reply.Total = total
	
	for _, v := range items {
		reply.Items = append(reply.Items, &filscan.FnsTransfer{
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

func (f FnsBiz) FnsControllers(ctx context.Context, request *filscan.FnsOwnersRequest) (reply *filscan.FnsOwnersReply, err error) {
	reply = &filscan.FnsOwnersReply{}
	items, total, err := f.repo.GetFnsRegistrants(ctx, filscan.PagingQuery{
		Index: request.Index,
		Limit: request.Limit,
		Order: nil,
	}, providers.ToAlias(request.Provider))
	if err != nil {
		return
	}
	
	reply.Total = total
	
	for _, v := range items {
		reply.Items = append(reply.Items, &filscan.FnsOwner{
			Rank:       v.Rank,
			Controller: v.Registrant,
			Amount:     v.Tokens,
			Percentage: v.Percent,
		})
	}
	return
}

func (f FnsBiz) FnsDomainDetail(ctx context.Context, request *filscan.FnsDetailRequest) (reply *filscan.FnsDetailReply, err error) {
	reply = &filscan.FnsDetailReply{}
	
	token, err := f.repo.GetFnsTokenOrNil(ctx, request.Domain, providers.ToAlias(request.Provider))
	if err != nil {
		return
	}
	if token == nil {
		return
	}
	
	reply = &filscan.FnsDetailReply{
		ResolvedAddress: token.FilAddress,
		ExpiredAt:       token.ExpiredAt,
		Registrant:      token.Controller,
		Controller:      token.Registrant,
		Exists:          true,
		IconUrl:         providers.GetLogo(request.Provider),
	}
	
	return
}

func (f FnsBiz) toDomainItem(name, provider string) *filscan.FnsDomainsReplyItem {
	
	p := providers.GetProvider(provider)
	
	return &filscan.FnsDomainsReplyItem{
		Domain:   name,
		Provider: providers.ToContract(provider),
		Name:     p.Name,
		LOGO:     p.LOGO,
	}
}

func (f FnsBiz) FnsAddressDomains(ctx context.Context, request *filscan.FnsAddressDomainsRequest) (reply *filscan.FnsControllerDomainsReply, err error) {
	reply = &filscan.FnsControllerDomainsReply{
		Registrant:      "",
		ResolvedAddress: "",
		Domains:         []*filscan.FnsDomainsReplyItem{},
	}
	
	var items []*bo.FnsOwnerToken
	
	switch request.Type {
	case "registrant":
		items, err = f.repo.GetFnsRegistrantTokens(ctx, request.Address)
	case "controller":
		items, err = f.repo.GetFnsControllerTokens(ctx, request.Address)
	default:
		err = fmt.Errorf("unsupported type: %s", request.Type)
		
	}
	if err != nil {
		return
	}
	for _, v := range items {
		reply.Domains = append(reply.Domains, f.toDomainItem(v.Name, v.Provider))
	}
	
	return
}

func (f FnsBiz) FnsBindDomains(ctx context.Context, request *filscan.FnsBindDomainsRequest) (reply filscan.FnsBindDomainsReply, err error) {
	
	reply.Provider = providers.GetProvider(providers.Filfox).Contract
	reply.Domains = map[string]string{}
	for _, v := range request.Addresses {
		addr := chain.SmartAddress(v)
		if addr.IsEthAddress() {
			reply.Domains[v] = ""
		} else if addr.IsContract() {
			reply.Domains[addr.Address()] = ""
		}
	}
	
	if len(reply.Domains) == 0 {
		return
	}
	type Result struct {
		Addr   string
		Domain string
	}
	
	var runners []parallel.Runner[Result]
	
	for k := range reply.Domains {
		r := Result{
			Addr: k,
		}
		runners = append(runners, func(ctx context.Context) (Result, error) {
			domain, e := f.task.GetFilfoxAddressDomain(ctx, chain.SmartAddress(r.Addr))
			if e != nil {
				return r, e
			}
			r.Domain = domain
			return r, nil
		})
	}
	
	ch := parallel.Run[Result](ctx, 3, runners)
	err = parallel.Wait[Result](ch, func(v Result) error {
		if v.Domain != "" {
			reply.Domains[v.Addr] = v.Domain
		} else {
			delete(reply.Domains, v.Addr)
		}
		return nil
	})
	if err != nil {
		return
	}
	
	return
}
