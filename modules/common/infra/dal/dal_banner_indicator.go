package dal

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

var _ repository.BannerIndicatorRepo = (*BannerIndicatorDal)(nil)

type BannerIndicatorDal struct {
	*_dal.BaseDal
}

func NewBannerIndicatorDal(db *gorm.DB) *BannerIndicatorDal {
	return &BannerIndicatorDal{BaseDal: _dal.NewBaseDal(db)}
}

func (b BannerIndicatorDal) GetTotalBalance(ctx context.Context) (res *decimal.Decimal, err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	totalBalance := &decimal.Decimal{}
	sql := `
			SELECT sum(actor_balance)
			FROM fevm.evm_transfer_stats
			WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transfer_stats WHERE interval = '1h')
			`
	err = tx.Raw(sql).Scan(totalBalance).Error
	return totalBalance, err
}

func (b BannerIndicatorDal) GetMinerPowerProportion(ctx context.Context) ([]*bo.MinerCount, error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return nil, err
	}
	mi := &bo.MinerInfo{}
	err = tx.Raw(`select max(epoch) as epoch from chain.miner_infos`).Scan(mi).Error
	if err != nil {
		return nil, err
	}

	var result []*bo.MinerCount
	for _, size := range []int64{32, 64} {
		var res bo.MinerCount
		sectorSize := size << 30
		sql := fmt.Sprintf(`select SUM(quality_adj_power) as quality_adj_power from chain.miner_infos where sector_size = %d and epoch = %d`,
			sectorSize, mi.Epoch)
		err = tx.Raw(sql).Scan(&res).Error
		if err != nil {
			return nil, err
		}
		res.SectorSize = sectorSize
		result = append(result, &res)
	}

	return result, err
}
