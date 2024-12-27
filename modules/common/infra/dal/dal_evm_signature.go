package dal

import (
	"context"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewEvmSignatureDal(db *gorm.DB) *EvmSignatureDal {
	return &EvmSignatureDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.EvmSignatureRepo = (*EvmSignatureDal)(nil)

type EvmSignatureDal struct {
	*_dal.BaseDal
}

func (e EvmSignatureDal) SaveEvmEventSignatures(ctx context.Context, infos []*po.EvmEventSignature) (err error) {
	err = e.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Debug().CreateInBatches(infos, 100).Error
	})
	return
}

func (e EvmSignatureDal) GetEvmEventSignatures(ctx context.Context, hexSignature []string) (signature []*po.EvmEventSignature, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where("hex_signature in ?", hexSignature).Find(&signature).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			signature = nil
		}
	}

	return
}
