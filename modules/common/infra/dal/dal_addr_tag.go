package dal

import (
	"context"

	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

func NewAddrTagDal(db *gorm.DB) *AddrTagDal {
	return &AddrTagDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.GetAddrTagRepo = (*AddrTagDal)(nil)

type AddrTagDal struct {
	*_dal.BaseDal
}

func (a AddrTagDal) GetAllAddrTags(ctx context.Context) ([]*po.AddressTag, error) {
	db, err := a.DB(ctx)
	if err != nil {
		return nil, err
	}
	items := []*po.AddressTag{}
	err = db.Find(&items).Error
	return items, err
}
