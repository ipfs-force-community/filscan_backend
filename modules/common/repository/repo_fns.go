package repository

import (
	"context"
	
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type NFTQueryer interface {
	GetFnsSummary(ctx context.Context, provider string) (item *bo.FnsSummary, err error)
	GetFnsTransfers(ctx context.Context, query filscan.PagingQuery, provider string) (items []*po.FNSTransfer, total int64, err error)
	GetFnsRegistrants(ctx context.Context, query filscan.PagingQuery, provider string) (items []*bo.FnsRegistrant, total int64, err error)
	GetFnsControllerTokens(ctx context.Context, controller string) (items []*bo.FnsOwnerToken, err error)
	GetFnsRegistrantTokens(ctx context.Context, registrant string) (items []*bo.FnsOwnerToken, err error)
	GetFnsTokenOrNil(ctx context.Context, name string, provider string) (item *po.FNSToken, err error)
	SearchFnsTokens(ctx context.Context, name string) (items []*po.FNSToken, err error)
	GetFnsTransfersByCid(ctx context.Context, cid string) (items []*po.FNSTransfer, err error)
	GetNFTSummaries(ctx context.Context, query filscan.PagingQuery) (items []*po.NFTContract, total int64, err error)
	GetNFTSummary(ctx context.Context, contract string) (item *po.NFTContract, err error)
	GetNFTOwners(ctx context.Context, contract string, query filscan.PagingQuery) (items []*bo.NFTOwner, total int64, err error)
	GetNFTTransfers(ctx context.Context, contract string, query filscan.PagingQuery) (items []*bo.NFTTransfer, total int64, err error)
	GetNFTTransfersByCid(ctx context.Context, cid string) (items []*bo.NFTTransfer, err error)
}

type FnsSaver interface {
	GetEventsByEpoch(ctx context.Context, epoch chain.Epoch) (events []*po.FNSEvent, err error)
	GetTokenOrNil(ctx context.Context, name, provider string) (item *po.FNSToken, err error)
	GetActionsAfterEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.FNSAction, err error)
	AddToken(ctx context.Context, item ...*po.FNSToken) (err error)
	AddAction(ctx context.Context, item *po.FNSAction) (err error)
	AddTransfer(ctx context.Context, item *po.FNSTransfer) (err error)
	AddEvents(ctx context.Context, items []*po.FNSEvent) (err error)
	DeleteTokenByName(ctx context.Context, name, provider string) (err error)
	DeleteEventsAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error)
	DeleteTransferAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error)
	DeleteActionsAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error)
	GetFnsReserveByAddressOrNil(ctx context.Context, address string) (item *po.FnsReserve, err error)
	DeleteOriginReserve(ctx context.Context, addr, domain string) (err error)
	AddFNsReserveDomain(ctx context.Context, item *po.FnsReserve) (err error)
	AddFnsReserveDomainWithConflict(ctx context.Context, item *po.FnsReserve) (err error)
	DeleteFnsReservesAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error)
}
