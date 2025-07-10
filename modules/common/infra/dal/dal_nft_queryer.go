package dal

import (
	"context"
	"fmt"
	
	"github.com/pkg/errors"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

var _ repository.NFTQueryer = (*NFTQueryer)(nil)

type NFTQueryer struct {
	*_dal.BaseDal
}

var fnsContracts = []string{"0x45d9d6408d5159a379924cf423cb7e15c00fa81f", "0xb8d7ca6a3253c418e52087693ca688d3257d70d1"}

func (f NFTQueryer) GetNFTSummaries(ctx context.Context, query filscan.PagingQuery) (items []*po.NFTContract, total int64, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	if query.Limit > 100 {
		err = fmt.Errorf("invalid limit")
		return
	}
	
	err = tx.Table("fevm.nft_contracts").
		Where("contract not in ? and logo != ''", fnsContracts).Count(&total).Error
	if err != nil {
		return
	}
	
	err = tx.Where("contract not in ? and logo != ''", fnsContracts).
		Limit(query.Limit).Offset(query.Limit * query.Index).
		Order("owners desc, transfers desc").Find(&items).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetNFTSummary(ctx context.Context, contract string) (item *po.NFTContract, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	item = new(po.NFTContract)
	err = tx.Where("contract = ?", contract).First(item).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetNFTOwners(ctx context.Context, contract string, query filscan.PagingQuery) (items []*bo.NFTOwner, total int64, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	if query.Limit > 100 {
		err = fmt.Errorf("invalid limit")
		return
	}
	
	err = tx.Raw(`
	with a as (
    select count(1) as total
    from fevm.nft_tokens
	where contract = ?
	)
	select b.*, (b.tokens::decimal / a.total) as percent
	from (select owner, count(1) as tokens, row_number() over (order by count(1) desc ) as rank
	      from fevm.nft_tokens
		  where contract = ?
	      group by owner
	      order by tokens desc) b,
	     a limit ? offset ?`, contract, contract, query.Limit, query.Index).Scan(&items).Error
	if err != nil {
		return
	}
	
	err = tx.Raw(`select count(distinct owner) from fevm.nft_tokens where contract = ?`, contract).Scan(&total).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetNFTTransfers(ctx context.Context, contract string, query filscan.PagingQuery) (items []*bo.NFTTransfer, total int64, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	if query.Limit > 100 {
		err = fmt.Errorf("invalid limit")
		return
	}
	
	err = tx.Table("fevm.nft_transfers").Where("contract = ?", contract).Count(&total).Error
	if err != nil {
		return
	}
	
	type Record struct {
		bo.NFTTransfer
		MethodName string
		string
	}
	
	var records []*Record
	err = tx.Raw(`SELECT a.*, coalesce(b.decode, a.method) as method_name,c.token_uri,c.token_url
		FROM "fevm"."nft_transfers" a
		         left join fevm.methods b on a.method = b.hex_signature
		         left join fevm.nft_tokens c on a.contract = c.contract and a.token_id = c.token_id
		WHERE a.contract = ?
		ORDER BY epoch desc
		LIMIT ? OFFSET ?`, contract, query.Limit, query.Limit*query.Index).Find(&records).Error
	if err != nil {
		return
	}
	
	for _, v := range records {
		v.NFTTransfer.Method = v.MethodName
		items = append(items, &v.NFTTransfer)
	}
	
	return
}

func (f NFTQueryer) SearchFnsTokens(ctx context.Context, name string) (items []*po.FNSToken, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Where("name = ?", name).Find(&items).Error
	if err != nil {
		return
	}
	
	return
}

func NewNFTQueryer(db *gorm.DB) *NFTQueryer {
	return &NFTQueryer{BaseDal: _dal.NewBaseDal(db)}
}

func (f NFTQueryer) GetFnsSummary(ctx context.Context, provider string) (item *bo.FnsSummary, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	item = new(bo.FnsSummary)
	
	err = tx.Raw(`select count(1) from fns.tokens where provider = ?`, provider).Scan(&item.Tokens).Error
	if err != nil {
		return
	}
	err = tx.Raw(`select count(1) from fns.transfers where provider = ?`, provider).Scan(&item.Transfers).Error
	if err != nil {
		return
	}
	err = tx.Raw(`select count(distinct registrant) from fns.tokens where provider = ?`, provider).Scan(&item.Controllers).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetFnsTransfers(ctx context.Context, query filscan.PagingQuery, provider string) (items []*po.FNSTransfer, total int64, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	if query.Limit > 100 {
		err = fmt.Errorf("invalid limit")
		return
	}
	
	err = tx.Where("provider = ?", provider).Limit(query.Limit).Offset(query.Limit * query.Index).Order("epoch desc").Find(&items).Error
	if err != nil {
		return
	}
	
	err = tx.Raw(`select count(1) from fns.transfers where provider=?`, provider).Scan(&total).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetFnsRegistrants(ctx context.Context, query filscan.PagingQuery, provider string) (items []*bo.FnsRegistrant, total int64, err error) {
	
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
	with a as (
    select count(1) as total
    from fns.tokens
	where provider = ?
	)
	select b.*, (b.tokens::decimal / a.total) as percent
	from (select registrant, count(1) as tokens, row_number() over (order by count(1) desc ) as rank
	      from fns.tokens
		  where provider = ?
	      group by registrant  
	      order by tokens desc) b,
	     a limit ? offset ?
	`, provider, provider, query.Limit, query.Limit*query.Index).Find(&items).Error
	if err != nil {
		return
	}
	
	err = tx.Raw(`select count(distinct registrant) from fns.tokens where provider = ?`, provider).Scan(&total).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetFnsControllerTokens(ctx context.Context, controller string) (items []*bo.FnsOwnerToken, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
		select name,provider from fns.tokens where controller = ?
		                            and expired_at > (SELECT EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INT)
   `, controller).Find(&items).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetFnsRegistrantTokens(ctx context.Context, registrant string) (items []*bo.FnsOwnerToken, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
		select name,provider from fns.tokens where registrant = ?
		                            and expired_at > (SELECT EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INT)
   `, registrant).Find(&items).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetFnsTokenOrNil(ctx context.Context, name string, provider string) (item *po.FNSToken, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	item = new(po.FNSToken)
	err = tx.Where("provider = ? and name= ?", provider, name).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
			return
		}
		return
	}
	
	return
}

func (f NFTQueryer) GetFnsTransfersByCid(ctx context.Context, cid string) (items []*po.FNSTransfer, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Where("cid = ?", cid).Find(&items).Error
	if err != nil {
		return
	}
	
	return
}

func (f NFTQueryer) GetNFTTransfersByCid(ctx context.Context, cid string) (items []*bo.NFTTransfer, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
	SELECT *, c.token_uri, c.token_url
	FROM "fevm"."nft_transfers" a
	         left join fevm.nft_tokens c on a.contract = c.contract and a.token_id = c.token_id
	WHERE cid = ?
	  and a.contract not in ?
   `, cid, fnsContracts).Find(&items).Error
	if err != nil {
		return
	}
	
	return
}
