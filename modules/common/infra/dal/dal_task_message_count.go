package dal

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMessageCountTaskDal(db *gorm.DB) *MessageCountTaskDal {
	return &MessageCountTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MessageCountTaskRepo = (*MessageCountTaskDal)(nil)

type MessageCountTaskDal struct {
	*_dal.BaseDal
}

func (m MessageCountTaskDal) GetAvgBlockCount24h(ctx context.Context) (count decimal.Decimal, err error) {
	
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`select (greatest(sum(avg_block_message * block) / 2880, 0))::decimal
	  from (select *
      from chain.message_counts
      order by epoch desc
      limit 2880) a;`).Scan(&count).Error
	if err != nil {
		return
	}
	
	return
}

func (m MessageCountTaskDal) SaveMessageCounts(ctx context.Context, count *po.MessageCount) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Create(count).Error
	
	return
}

func (m MessageCountTaskDal) DeleteMessageCounts(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Exec(`delete from chain.message_counts where epoch >= ?`, gteEpoch.Int64()).Error
	
	return
}
