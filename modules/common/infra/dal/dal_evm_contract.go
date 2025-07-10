package dal

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewEvmContractDal(db *gorm.DB) *EvmContractDal {
	return &EvmContractDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.EvmContractRepo = (*EvmContractDal)(nil)

type EvmContractDal struct {
	*_dal.BaseDal
}

func (c EvmContractDal) SaveFEvmContracts(ctx context.Context, item *po.FEvmContracts) (err error) {

	db, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = db.Create(item).Error
	if err != nil {
		return
	}
	return
}

func (c EvmContractDal) SelectFEvmContractsByActorID(ctx context.Context, actorID string) (item *po.FEvmContracts, err error) {

	db, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = db.Where("actor_id = ?", actorID).
		First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}
	return
}

func (c EvmContractDal) SaveFEvmContractSols(ctx context.Context, item []*po.FEvmContractSols) (err error) {

	db, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = db.CreateInBatches(&item, 100).Error
	if err != nil {
		return
	}
	return
}

func (c EvmContractDal) SelectFEvmContractSolsByActorID(ctx context.Context, actorID string) (item []*po.FEvmContractSols, err error) {

	db, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = db.Where("actor_id = ?", actorID).
		Find(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}
	return
}

func (c EvmContractDal) SelectFEvmMainContractByActorID(ctx context.Context, actorID string) (item *po.FEvmContractSols, err error) {

	db, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = db.Where("actor_id = ? AND is_main_contract = true", actorID).
		Find(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}
	return
}

func (c EvmContractDal) SelectVerifiedFEvmContracts(ctx context.Context, index *int, limit *int) (items []*bo.VerifiedContracts, total int64, err error) {

	db, err := c.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT fc.actor_id         AS actor_id,
       fc.actor_address    AS actor_address,
       fc.contract_address AS contract_address,
       fc.arguments        AS arguments,
       fc.license          AS license,
       fc.language         AS language,
       fc.compiler         AS compiler,
       fc.optimize         AS optimize,
       fc.optimize_runs    AS optimize_runs,
       fcs.contract_name   AS contract_name,
       fcs.byte_code       As byte_code,
       fcs.abi             As abi,
       fcs.created_at      As created_at
FROM fevm.contracts fc
         LEFT JOIN fevm.contract_sols fcs ON fc.actor_id = fcs.actor_id
WHERE fcs.is_main_contract = true
ORDER BY fc.created_at DESC
`
	err = db.Find(&po.FEvmContracts{}).Count(&total).Error
	if err != nil {
		return
	}

	if index != nil && limit != nil {
		newPage := *index
		newLimit := *limit
		offsetSize := fmt.Sprintf("OFFSET %d\n", newPage*newLimit)
		limitSize := fmt.Sprintf("LIMIT %d\n", newLimit)
		sql = sql + offsetSize + limitSize

		err = db.Debug().Raw(sql).
			Find(&items).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = nil
				items = nil
			}
			return
		}
	} else {
		err = db.Raw(sql).
			Find(&items).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = nil
				items = nil
			}
			return
		}
	}

	return
}
