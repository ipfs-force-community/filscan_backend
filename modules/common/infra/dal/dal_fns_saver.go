package dal

import (
	"context"
	
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewFnsSaverDal(db *gorm.DB) *FnsSaverDal {
	return &FnsSaverDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.FnsSaver = (*FnsSaverDal)(nil)

type FnsSaverDal struct {
	*_dal.BaseDal
}

func (f FnsSaverDal) AddTransfer(ctx context.Context, item *po.FNSTransfer) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Create(item).Error
	return
}

func (f FnsSaverDal) DeleteTokenByName(ctx context.Context, name, provider string) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Exec(`delete from fns.tokens where name=? and provider=?`, name, provider).Error
	if err != nil {
		return
	}
	
	return
}

func (f FnsSaverDal) DeleteEventsAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fns.events where epoch >= ?`, epoch.Int64()).Error
	return
}

func (f FnsSaverDal) DeleteTransferAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fns.transfers where epoch >= ?`, epoch.Int64()).Error
	return
}

func (f FnsSaverDal) DeleteActionsAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fns.actions where epoch >= ?`, epoch.Int64()).Error
	return
}

func (f FnsSaverDal) GetEventsByEpoch(ctx context.Context, epoch chain.Epoch) (events []*po.FNSEvent, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch = ?", epoch.Int64()).Find(&events).Error
	return
}

func (f FnsSaverDal) AddToken(ctx context.Context, item ...*po.FNSToken) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(item, 100).Error
	return
}

func (f FnsSaverDal) AddEvents(ctx context.Context, items []*po.FNSEvent) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(items, 100).Error
	return
}

func (f FnsSaverDal) GetTokenOrNil(ctx context.Context, name, provider string) (item *po.FNSToken, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	item = new(po.FNSToken)
	err = tx.Where("name = ? and provider = ?", name, provider).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}
	
	return
}

func (f FnsSaverDal) AddAction(ctx context.Context, item *po.FNSAction) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Create(item).Error
	return
}

func (f FnsSaverDal) GetActionsAfterEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.FNSAction, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch >= ?", epoch.Int64()).Find(&items).Error
	return
}

func (f FnsSaverDal) GetFnsReserveByAddressOrNil(ctx context.Context, address string) (item *po.FnsReserve, err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	item = new(po.FnsReserve)
	err = tx.Where("address = ?", address).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}
	
	return
}

func (f FnsSaverDal) DeleteOriginReserve(ctx context.Context, addr, domain string) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Exec("delete from fns.reverses where address = ? or domain = ?", addr, domain).Error
	if err != nil {
		return
	}
	
	return
}

func (f FnsSaverDal) AddFNsReserveDomain(ctx context.Context, item *po.FnsReserve) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Create(item).Error
	if err != nil {
		return
	}
	return
}

func (f FnsSaverDal) DeleteFnsReservesAfterEpoch(ctx context.Context, epoch chain.Epoch) (err error) {
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw("delete from fns.reverses where epoch >= ?", epoch.Int64()).Error
	if err != nil {
		return
	}
	
	return
}

func (f FnsSaverDal) AddFnsReserveDomainWithConflict(ctx context.Context, item *po.FnsReserve) (err error) {
	
	tx, err := f.DB(ctx)
	if err != nil {
		return
	}
	
	if item.Domain == nil {
		return
	}
	
	err = tx.Exec("insert into fns.reverses(address, domain, epoch) values (?,?,?) on conflict(domain) do update set address = ?,epoch = ?",
		item.Address, item.Domain, item.Epoch, item.Address, item.Epoch).Error
	if err != nil {
		return
	}
	
	return
}
