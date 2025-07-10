package dal

import (
	"context"
	"github.com/shopspring/decimal"
	
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewSyncerTraceTaskDal(db *gorm.DB) *SyncerTraceTaskDal {
	return &SyncerTraceTaskDal{
		BaseDal:     _dal.NewBaseDal(db),
		ActorGetter: NewActorGetter(db),
	}
}

var _ repository.SyncerTraceTaskRepo = (*SyncerTraceTaskDal)(nil)

type SyncerTraceTaskDal struct {
	*_dal.BaseDal
	*ActorGetter
}

func (s SyncerTraceTaskDal) SaveMinerGasFees(ctx context.Context, items []*po.MinerGasFee) (err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	err = db.CreateInBatches(items, 10).Error
	if err != nil {
		return
	}
	return
}

func (s SyncerTraceTaskDal) GetLastBaseGasCostOrNil(ctx context.Context, epoch chain.Epoch) (base *stat.BaseGasCost, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	item := &po.BaseGasCostPo{}
	err = db.Where("epoch < ?", epoch.Int64()).Order("epoch desc").First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	
	c := convertor.BaseGasCostConvertor{}
	base, err = c.ToBaseGasCostEntity(item)
	if err != nil {
		return
	}
	
	return
}

func (s SyncerTraceTaskDal) SaveMethodGasFees(ctx context.Context, entities []*po.MethodGasFee) (err error) {
	
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	err = db.CreateInBatches(entities, 10).Error
	if err != nil {
		return
	}
	
	return
}

func (s SyncerTraceTaskDal) SaveBaseGasCost(ctx context.Context, data *stat.BaseGasCost) (err error) {
	
	db, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	c := convertor.BaseGasCostConvertor{}
	item, err := c.ToBaseGasCostPo(data)
	if err != nil {
		return
	}
	
	err = db.Create(item).Error
	if err != nil {
		return
	}
	
	return
}

func (s SyncerTraceTaskDal) DeleteBaseGasCosts(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.base_gas_costs where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (s SyncerTraceTaskDal) DeleteMethodGasFees(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.method_gas_fees where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (s SyncerTraceTaskDal) DeleteMinerGasFees(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_gas_fees where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (s SyncerTraceTaskDal) GetBaseGasCosts(ctx context.Context, gtStart, lteEnd chain.Epoch) (items []*po.BaseGasCostPo, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch > ? and epoch <= ?", gtStart.Int64(), lteEnd.Int64()).Order("epoch desc").Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (s SyncerTraceTaskDal) UpdateBaseGasCostSectorGas(ctx context.Context, epoch chain.Epoch, sectorFee32, sectorFee64 decimal.Decimal) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`update chain.base_gas_costs set sector_fee32 = ?, sector_fee64 = ? where epoch = ?`,
		sectorFee32, sectorFee64, epoch.Int64()).Error
	if err != nil {
		return
	}
	return
}
