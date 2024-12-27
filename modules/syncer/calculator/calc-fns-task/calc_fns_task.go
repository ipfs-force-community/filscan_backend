package calc_fns_task

import (
	"context"
	"fmt"
	"github.com/gozelle/mix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns/providers"
	"math/big"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/gozelle/spew"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"go.uber.org/zap"
)

type Cid = string

type EventName = string

func NewCalcFnsTask(abiDecoder fevm.ABIDecoderAPI, fnsSaver repository.FnsSaver) *CalcFnsTask {

	t := &CalcFnsTask{
		//node:       node,
		repo:       fnsSaver,
		abiDecoder: abiDecoder,
		filfox:     fns.NewFilfoxToken(),
		opengate:   fns.NewOpengateToken(),
	}

	return t
}

var _ syncer.Calculator = (*CalcFnsTask)(nil)

type CalcFnsTask struct {
	//node       *lotus_api.Node
	accept     []fns.Token
	repo       repository.FnsSaver
	abiDecoder fevm.ABIDecoderAPI
	store      *Store
	filfox     fns.Contract
	opengate   fns.Contract
	log        *zap.SugaredLogger
}

func (o CalcFnsTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	err = o.repo.DeleteActionsAfterEpoch(ctx, safeClearEpoch)
	if err != nil {
		return
	}
	return
}

func (o CalcFnsTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {

	actions, err := o.repo.GetActionsAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}

	for _, v := range actions {
		var token fns.Token
		switch v.Provider {
		case providers.Filfox:
			token = fns.NewFilfoxTokenWithDomain(o.abiDecoder, v.Name)
		case providers.Opengate:
			token = fns.NewOpengateTokenWithDomain(o.abiDecoder, v.Name)
		}
		var item *po.FNSToken
		item, err = o.prepareToken(token, v.Provider)
		if err != nil {
			return
		}
		if item == nil {
			continue
		}

		err = o.repo.DeleteTokenByName(ctx, item.Name, item.Provider)
		if err != nil {
			return
		}
		if v.Action == po.FNSActionUpdate {
			err = o.repo.AddToken(ctx, item)
			if err != nil {
				return
			}
		}
	}

	err = o.repo.DeleteActionsAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}

	err = o.repo.DeleteTransferAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}

	err = o.repo.DeleteFnsReservesAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}

	return
}

func (o CalcFnsTask) Name() string {
	return "calc-fns-task"
}

func (o CalcFnsTask) Calc(ctx *syncer.Context) (err error) {

	o.log = ctx.SugaredLogger
	o.store = NewStore(o.repo.(repository.FnsSaver))

	items, err := o.repo.GetEventsByEpoch(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}

	if len(items) == 0 {
		return
	}

	ctx.Infof("事件数: %d", len(items))
	abe, err := o.abiDecoder.ChainHead()
	if err != nil {
		return
	}

	if abe < ctx.Epoch() {
		err = mix.Warnf("解码器节点高度: %s 未到", abe)
		return
	}

	tokens := map[string]*po.FNSToken{}

	for _, event := range items {
		var filAddr address.Address
		var ethAddr ethtypes.EthAddress

		if strings.HasPrefix(event.Contract, "0x") {
			var a ethtypes.EthAddress
			a, err = ethtypes.ParseEthAddress(event.Contract)
			if err != nil {
				return
			}
			var b address.Address
			b, err = a.ToFilecoinAddress()
			if err != nil {
				return
			}
			event.Contract = b.String()
		}

		// 处理反向代理设置
		err = o.handleReserveClaimed(ctx, event)
		if err != nil {
			return
		}

		{
			ethAddr, err = ethtypes.ParseEthAddress(o.filfox.RegistrarContract())
			if err != nil {
				return
			}
			filAddr, err = ethAddr.ToFilecoinAddress()
			if err != nil {
				return
			}
			if event.Contract == filAddr.String() && event.EventName == "Transfer" {
				var vv *po.FNSToken
				vv, err = o.GetTokenByTokenId(&tokens, providers.Filfox, event.Topics[3])
				if err != nil {
					spew.Json(event)
					return
				}
				if vv != nil {
					err = o.saveTransfer(ctx.Context(), event, vv.Name, vv.Provider)
					if err != nil {
						return
					}
				}
			}
		}
		{
			ethAddr, err = ethtypes.ParseEthAddress(o.opengate.RegistrarContract())
			if err != nil {
				return
			}
			filAddr, err = ethAddr.ToFilecoinAddress()
			if err != nil {
				return
			}
			if event.Contract == filAddr.String() && event.EventName == "Transfer" {
				var vv *po.FNSToken
				vv, err = o.GetTokenByTokenId(&tokens, providers.Opengate, event.Topics[3])
				if err != nil {
					spew.Json(event)
					return
				}
				if vv != nil {
					err = o.saveTransfer(ctx.Context(), event, vv.Name, vv.Provider)
					if err != nil {
						return
					}
				}
			}
		}
		{
			ethAddr, err = ethtypes.ParseEthAddress(o.filfox.RegistrarContract())
			if err != nil {
				return
			}
			filAddr, err = ethAddr.ToFilecoinAddress()
			if err != nil {
				return
			}
			if event.Contract == filAddr.String() && event.EventName == "NameRenewed" {
				_, err = o.GetTokenByTokenId(&tokens, providers.Filfox, event.Topics[1])
				if err != nil {
					return
				}
			}
		}
		{
			ethAddr, err = ethtypes.ParseEthAddress(o.opengate.RegistrarContract())
			if err != nil {
				return
			}
			filAddr, err = ethAddr.ToFilecoinAddress()
			if err != nil {
				return
			}
			if event.Contract == filAddr.String() && event.EventName == "NameRenewed" {
				_, err = o.GetTokenByTokenId(&tokens, providers.Opengate, event.Topics[1])
				if err != nil {
					return
				}
			}
		}
		{
			ethAddr, err = ethtypes.ParseEthAddress(o.filfox.PublicResolverContract())
			if err != nil {
				return
			}
			filAddr, err = ethAddr.ToFilecoinAddress()
			if err != nil {
				return
			}
			if event.Contract == filAddr.String() && event.EventName == "AddressChanged" {
				err = o.GetTokenByTokenNode(&tokens, providers.Filfox, event.Topics[1])
				if err != nil {
					return
				}
			}
		}
		{
			ethAddr, err = ethtypes.ParseEthAddress(o.opengate.PublicResolverContract())
			if err != nil {
				return
			}
			filAddr, err = ethAddr.ToFilecoinAddress()
			if err != nil {
				return
			}
			if event.Contract == filAddr.String() && event.EventName == "AddressChanged" {
				err = o.GetTokenByTokenNode(&tokens, providers.Opengate, event.Topics[1])
				if err != nil {
					return
				}
			}
		}
	}

	var tokenItems []*po.FNSToken
	for _, v := range tokens {
		v.LastEventEpoch = ctx.Epoch().Int64()
		tokenItems = append(tokenItems, v)
	}

	err = o.store.SaveToken(ctx.Context(), ctx.Epoch().Int64(), tokenItems)
	if err != nil {
		return
	}

	return
}

func (o CalcFnsTask) handleReserveClaimed(ctx *syncer.Context, event *po.FNSEvent) (err error) {

	ethAddr, err := ethtypes.ParseEthAddress(o.filfox.ReverseRegistrarContract())
	if err != nil {
		return
	}
	filAddr, err := ethAddr.ToFilecoinAddress()
	if event.Contract != filAddr.String() || event.EventName != "ReverseClaimed" {
		return
	}

	var reverseAddr string
	reverseAddr, err = o.abiDecoder.DecodeEventHexAddress(event.Topics[1])
	if err != nil {
		return
	}

	reverseAddr = strings.ToLower(reverseAddr)

	domain, err := o.abiDecoder.FNSNodeName(fevm.Contract{
		ABI:     o.filfox.PublicResolverABI(),
		Address: o.filfox.PublicResolverContract(),
	}, event.Topics[2])
	if err != nil {
		return
	}

	err = o.repo.DeleteOriginReserve(ctx.Context(), reverseAddr, domain)
	if err != nil {
		return
	}

	if domain == "" {
		return
	}

	err = o.repo.AddFNsReserveDomain(ctx.Context(), &po.FnsReserve{
		Address: reverseAddr,
		Domain:  &domain,
		Epoch:   ctx.Epoch().Int64(),
	})
	if err != nil {
		return
	}

	return
}

func (o CalcFnsTask) GetFilfoxAddressDomain(ctx context.Context, addr chain.SmartAddress) (domain string, err error) {

	if !addr.IsEthAddress() {
		var a address.Address
		a, err = address.NewFromString(addr.Address())
		if err != nil {
			return
		}
		var ea ethtypes.EthAddress
		ea, err = ethtypes.EthAddressFromFilecoinAddress(a)
		if err != nil {
			return
		}
		addr = chain.SmartAddress(ea.String())
	}

	addr = chain.SmartAddress(strings.ToLower(string(addr)))

	item, err := o.repo.GetFnsReserveByAddressOrNil(ctx, string(addr))
	if err != nil {
		return
	}

	if item != nil {
		if item.Domain != nil {
			domain = *item.Domain
		}
		return
	}

	r, err := o.QueryFilfoxAddressDomain(addr)
	if err != nil {
		return
	}

	item = &po.FnsReserve{
		Address: string(addr),
		Domain:  nil,
		Epoch:   chain.CurrentEpoch().Int64(),
	}
	if r == "" {
		err = o.repo.AddFnsReserveDomainWithConflict(ctx, item)
		if err != nil {
			return
		}
		return
	} else {
		var domainNode string
		domainNode, err = o.abiDecoder.FNSTokenNode(r)
		if err != nil {
			return
		}
		var controller string
		controller, err = o.abiDecoder.FNSOwner(fevm.Contract{
			ABI:     o.filfox.FNSRegistryABI(),
			Address: o.filfox.FNSRegistryContract(),
		}, domainNode)
		if err != nil {
			return
		}
		if strings.ToLower(controller) != string(addr) {
			err = o.repo.AddFnsReserveDomainWithConflict(ctx, item)
			if err != nil {
				return
			}
			return
		}
	}

	item.Domain = &r
	err = o.repo.AddFnsReserveDomainWithConflict(ctx, item)
	if err != nil {
		return
	}

	domain = r

	return
}

func (o CalcFnsTask) QueryFilfoxAddressDomain(addr chain.SmartAddress) (domain string, err error) {

	if addr.IsEthAddress() {
		b := &big.Int{}
		b.SetString(strings.TrimPrefix(string(addr), "0x"), 16)
		if b.Cmp(big.NewInt(0)) == 0 {
			return
		}
	}

	if addr.IsContract() {
		var a address.Address
		a, err = address.NewFromString(addr.Address())
		if err != nil {
			return
		}
		var ea ethtypes.EthAddress
		ea, err = ethtypes.EthAddressFromFilecoinAddress(a)
		if err != nil {
			return
		}
		addr = chain.SmartAddress(ea.String())
	}

	node, err := o.abiDecoder.FNSNode(fevm.Contract{
		ABI:     o.filfox.ReverseRegistrarABI(),
		Address: o.filfox.ReverseRegistrarContract(),
	}, string(addr))
	if err != nil {
		err = fmt.Errorf("query node error: %s", err)
		return
	}

	domain, err = o.abiDecoder.FNSNodeName(fevm.Contract{
		ABI:     o.filfox.PublicResolverABI(),
		Address: o.filfox.PublicResolverContract(),
	}, node)
	if err != nil {
		return
	}

	return
}

func (o CalcFnsTask) saveTransfer(ctx context.Context, event *po.FNSEvent, item, provider string) (err error) {
	err = o.store.SaveTransfer(ctx, &po.FNSTransfer{
		Epoch:    event.Epoch,
		Cid:      event.Cid,
		Provider: provider,
		LogIndex: event.LogIndex,
		Method:   event.MethodName,
		From:     event.Topics[1],
		To:       event.Topics[2],
		TokenId:  event.Topics[3],
		Contract: event.Contract,
		Item:     item,
	})
	if err != nil {
		return
	}
	return
}

func (o CalcFnsTask) GetTokenByTokenId(tokens *map[string]*po.FNSToken, provider string, tokenId string) (*po.FNSToken, error) {

	var token fns.Token
	switch provider {
	case providers.Filfox:
		token = fns.NewFilfoxTokenWithTokenID(o.abiDecoder, tokenId)
	case providers.Opengate:
		token = fns.NewOpengateTokenWithTokenID(o.abiDecoder, tokenId)
	}

	v, err := o.tokenExists(tokens, token)
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}

	return o.registerToken(tokens, token, provider)
}

func (o CalcFnsTask) GetTokenByTokenNode(tokens *map[string]*po.FNSToken, provider string, node string) error {
	var token fns.Token
	switch provider {
	case providers.Filfox:
		token = fns.NewFilfoxTokenWithNode(o.abiDecoder, node)
	case providers.Opengate:
		token = fns.NewOpengateTokenWithNode(o.abiDecoder, node)
	}

	// 根据 Node 反推 Controller 去找 Domain 时，有可能域名已经转移了
	name, _ := token.Domain()
	if strings.TrimSuffix(name, ".fil") == "" {
		return nil
	}

	v, err := o.tokenExists(tokens, token)
	if err != nil {
		return err
	}
	if v != nil {
		return nil
	}

	_, err = o.registerToken(tokens, token, provider)
	return err
}

func (o CalcFnsTask) tokenExists(tokens *map[string]*po.FNSToken, token fns.Token) (*po.FNSToken, error) {

	key, err := token.Domain()
	if err != nil {
		return nil, err
	}

	return (*tokens)[key], nil
}

func (o CalcFnsTask) registerToken(tokens *map[string]*po.FNSToken, token fns.Token, provider string) (item *po.FNSToken, err error) {
	item, err = o.prepareToken(token, provider)
	if err != nil {
		return
	}
	if item != nil {
		(*tokens)[item.Name] = item
	}
	return
}

func (o CalcFnsTask) prepareToken(token fns.Token, provider string) (item *po.FNSToken, err error) {

	item = &po.FNSToken{
		Name:           "",
		TokenId:        "",
		Provider:       provider,
		Node:           "",
		Registrant:     "",
		Controller:     "",
		ExpiredAt:      0,
		FilAddress:     "",
		LastEventEpoch: 0,
	}

	item.Name, err = token.Domain()
	if err != nil {
		return
	}

	item.TokenId, err = token.TokenId()
	if err != nil {
		return
	}

	item.Node, err = token.Node()
	if err != nil {
		o.log.Warnf("忽略非法域名解析: %s", item.Name)
		item = nil
		err = nil
		return //nolint
	}

	item.Registrant, err = token.Registrant()
	if err != nil {
		return
	}

	item.Controller, err = token.Controller()
	if err != nil {
		return
	}

	item.ExpiredAt, err = token.ExpiredAt()
	if err != nil {
		return
	}

	item.FilAddress, err = token.FilResolved()
	if err != nil {
		return
	}

	return
}
