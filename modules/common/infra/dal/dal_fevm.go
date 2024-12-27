package dal

import (
	"context"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewFEvmDal(db *gorm.DB) *FEvmDal {
	return &FEvmDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.FEvmRepo = (*FEvmDal)(nil)

type FEvmDal struct {
	*_dal.BaseDal
}

func (c FEvmDal) CreateERC20TransferBatch(ctx context.Context, items []*po.FEvmERC20Transfer) (err error) {
	db, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = db.CreateInBatches(items, 100).Error
	return
}

func (c FEvmDal) CreateErc721TransferBatch(ctx context.Context, items []*po.NFTTransfer) (err error) {
	db, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = db.CreateInBatches(items, 100).Error
	return
}

func (c FEvmDal) CreateErc721Tokens(ctx context.Context, items []*po.NFTToken) (err error) {
	db, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = db.CreateInBatches(items, 100).Error
	return
}

func (c FEvmDal) SaveAPISignatures(ctx context.Context, items []*po.FEvmABISignature) (err error) {
	db, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "type"},
			{Name: "id"},
		},
		DoNothing: true}).
		CreateInBatches(items, 100).Error
	return
}

func (c FEvmDal) GetMethodNameBySignature(ctx context.Context, sig string) (name string, err error) {
	
	db, err := c.DB(ctx)
	if err != nil {
		return
	}
	
	item := new(po.FEvmABISignature)
	
	err = db.Where("type = 'method' and id = ?", sig).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	
	name = item.Name
	
	return
}

func (c FEvmDal) GetEventNameBySignature(ctx context.Context, sig string) (name string, err error) {
	
	db, err := c.DB(ctx)
	if err != nil {
		return
	}
	
	item := new(po.FEvmABISignature)
	
	err = db.Where("type = 'event' and id = ?", sig).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	
	name = item.Name
	
	return
}
