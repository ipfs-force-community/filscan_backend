package dal

import (
	"context"

	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

func NewEventsDal(db *gorm.DB) *EventsDal {
	return &EventsDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.EventsRepo = (*EventsDal)(nil)

type EventsDal struct {
	*_dal.BaseDal
}

func (e EventsDal) GetEventsList(ctx context.Context) (items []*po.Events, err error) {
	db, err := e.DB(ctx)
	if err != nil {
		return
	}

	err = db.Find(&items).Order("end_at desc").Error
	return items, err
}
