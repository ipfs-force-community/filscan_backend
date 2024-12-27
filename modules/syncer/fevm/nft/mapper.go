package nft

import (
	"context"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type iMapper interface {
	GetTransfers(ctx context.Context, epochs chain.LCRCRange) (items []*po.NFTTransfer, err error)
	GetTransfersAfterEpoch(ctx context.Context, gteEpoch chain.Epoch) (items []*po.NFTTransfer, err error)
	SaveTokens(ctx context.Context, items []*po.NFTToken) (err error)
	SaveTransfers(ctx context.Context, items []*po.NFTTransfer) (err error)
	SaveContracts(ctx context.Context, items []*po.NFTContract) (err error)
	DeleteToken(ctx context.Context, contract, tokenId string) (err error)
	DeleteTransfersAfterEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error)
	CountContractMints(ctx context.Context, contract string) (count int64, err error)
	CountContractOwners(ctx context.Context, contract string) (count int64, err error)
	CountContractTransfers(ctx context.Context, contract string) (count int64, err error)
	UpdateContractCounts(ctx context.Context, contract string, mints, owners, transfers int64) (err error)
	GetContractCollection(ctx context.Context, contract string) (name string, err error)
}

func NewMapper(db *gorm.DB) *Mapper {
	return &Mapper{BaseDal: _dal.NewBaseDal(db)}
}

var (
	_ iMapper = (*Mapper)(nil)
)

type Mapper struct {
	*_dal.BaseDal
}

func (m Mapper) GetContractCollection(ctx context.Context, contract string) (name string, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	token := &po.NFTToken{}
	err = tx.Where("contract = ?", contract).First(token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	
	name = token.Name
	
	return
}

func (m Mapper) CountContractMints(ctx context.Context, contract string) (count int64, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`select count(1)
		from fevm.nft_tokens
		where contract = ?`, contract).Scan(&count).Error
	if err != nil {
		return
	}
	
	return
}

func (m Mapper) CountContractOwners(ctx context.Context, contract string) (count int64, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`select count(distinct owner)
			from fevm.nft_tokens
			where contract = ?`, contract).Scan(&count).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) CountContractTransfers(ctx context.Context, contract string) (count int64, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`select count(1)
			from fevm.nft_transfers
			where contract = ?`, contract).Scan(&count).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) UpdateContractCounts(ctx context.Context, contract string, mints, owners, transfers int64) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`update fevm.nft_contracts set mints = ?, owners = ?, transfers = ? where contract = ?`,
		mints, owners, transfers, contract).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) DeleteToken(ctx context.Context, contract, tokenId string) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.nft_tokens where contract = ? and token_id = ?`, contract, tokenId).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) DeleteTransfersAfterEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.nft_transfers where epoch >= ?`, gteEpoch.Int64()).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) GetTransfersAfterEpoch(ctx context.Context, gteEpoch chain.Epoch) (items []*po.NFTTransfer, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch >= ?", gteEpoch.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) GetTransfers(ctx context.Context, epochs chain.LCRCRange) (items []*po.NFTTransfer, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch >= ? and epoch <= ?", epochs.GteBegin.Int64(), epochs.LteEnd.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m Mapper) SaveContracts(ctx context.Context, items []*po.NFTContract) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "contract"},
		},
		DoNothing: true}).
		CreateInBatches(items, 5).Error
	return
}

func (m Mapper) SaveTokens(ctx context.Context, items []*po.NFTToken) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	for _, v := range items {
		sql := `insert into fevm.nft_tokens(token_id, contract, name, symbol, token_uri, token_url, owner,item)
				VALUES (?,?,?,?,?,?,?,?) on conflict(contract,token_id) do update set token_url = excluded.token_url, owner = excluded.owner`
		err = tx.Exec(sql, v.TokenId, v.Contract, v.Name, v.Symbol, v.TokenUri, v.TokenUrl, v.Owner, v.Item).Error
		if err != nil {
			return
		}
	}
	
	return
}

func (m Mapper) SaveTransfers(ctx context.Context, items []*po.NFTTransfer) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	c := map[int64]struct{}{}
	for _, v := range items {
		c[v.Epoch] = struct{}{}
	}
	
	var epochs []int64
	for k := range c {
		epochs = append(epochs, k)
	}
	
	var transfers []*po.NFTTransfer
	err = tx.Select("cid").Where("epoch in ?", epochs).Find(&transfers).Error
	if err != nil {
		return
	}
	
	cids := map[string]struct{}{}
	for _, v := range transfers {
		cids[v.Cid] = struct{}{}
	}
	
	for _, v := range items {
		if _, ok := cids[v.Cid]; ok {
			continue
		}
		err = tx.Create(v).Error
		if err != nil {
			return
		}
	}
	return
}
