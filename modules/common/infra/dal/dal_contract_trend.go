package dal

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewContractTrendBizDal(db *gorm.DB) *ContractTrendBizDal {
	return &ContractTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.ContractTrendBizRepo = (*ContractTrendBizDal)(nil)

type ContractTrendBizDal struct {
	*_dal.BaseDal
}

func (c ContractTrendBizDal) GetContractUsersByEpochs(ctx context.Context, points []chain.Epoch) (items []*filscan.ContractUsersTrend, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	for i := 1; i < len(points); i++ {
		res := int64(0)
		err = tx.Model(&po.EvmTransfer{}).Where("epoch > ? and epoch <= ?", points[i-1], points[i]).Distinct("user_address").Count(&res).Error
		if err != nil {
			return
		}
		items = append(items, &filscan.ContractUsersTrend{
			BlockTime:     points[i].Unix(),
			ContractUsers: res,
		})
	}
	return
}

func (c ContractTrendBizDal) GetContractCntByEpochs(ctx context.Context, points []chain.Epoch) (contractCntList []*bo.ContractCnt, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	contractCntList = make([]*bo.ContractCnt, len(points))
	sql := `
			SELECT epoch, count(*) as cnts
			FROM fevm.evm_transfer_stats
			WHERE epoch in ? and interval = '1h' group by epoch order by epoch asc
			`
	err = tx.Raw(sql, points).Scan(&contractCntList).Error
	return
}

func (c ContractTrendBizDal) GetContractTxsByEpochs(ctx context.Context, points []chain.Epoch) (items []*filscan.ContractTxsTrend, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	for i := 1; i < len(points); i++ {
		res := int64(0)
		err = tx.Model(&po.EvmTransfer{}).Where("epoch > ? and epoch <= ?", points[i-1], points[i]).Count(&res).Error
		if err != nil {
			return
		}
		items = append(items, &filscan.ContractTxsTrend{
			BlockTime:   points[i].Unix(),
			ContractTxs: res,
		})
	}
	return
}

func (c ContractTrendBizDal) GetContractBalanceByEpochs(ctx context.Context, points []chain.Epoch) (items []*filscan.ContractBalanceTrend, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	totalBalanceList := make([]bo.ContractTotalBalance, len(points))
	sql := `
			SELECT epoch, sum(actor_balance) as balance
			FROM fevm.evm_transfer_stats
			WHERE epoch in ? and interval = '1h' group by epoch order by epoch asc
			`
	err = tx.Raw(sql, points).Scan(&totalBalanceList).Error
	if err != nil {
		return
	}
	for i := 0; i < len(totalBalanceList); i++ {
		items = append(items, &filscan.ContractBalanceTrend{
			BlockTime:            totalBalanceList[i].Epoch.Unix(),
			ContractTotalBalance: totalBalanceList[i].Balance,
		})
	}
	return
}
