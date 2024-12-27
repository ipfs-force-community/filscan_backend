package dal

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewEvmTransactionDal(db *gorm.DB) *EvmTransactionDal {
	return &EvmTransactionDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.EvmTransactionRepo = (*EvmTransactionDal)(nil)

type EvmTransactionDal struct {
	*_dal.BaseDal
}

func (e EvmTransactionDal) GetEvmTransactionStatsByID(ctx context.Context, actorID string) (evmTransaction *po.EvmTransactionStat, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	evmTransaction = new(po.EvmTransactionStat)
	sql := `
SELECT *
FROM fevm.evm_transaction_stats
WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transaction_stats WHERE interval = '1h') AND actor_id = ?
`
	err = tx.Raw(sql, actorID).First(evmTransaction).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			evmTransaction = nil
			err = nil
		}
		return
	}
	return
}

func (e EvmTransactionDal) GetEvmTransactionStatsList(ctx context.Context, page, limit int, filed, sort, interval string) (transactions []*po.EvmTransactionStat, count int, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT *
FROM fevm.evm_transaction_stats
WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transaction_stats WHERE interval = '1h')
`
	if filed != "" && sort != "" {
		if filed == "transaction_count" {
			filed = "acc_transaction_count"
		}
		if filed == "user_count" {
			filed = "acc_user_count"
		}
		if filed == "gas_cost" {
			filed = "acc_gas_cost"
		}
		order := fmt.Sprintf("ORDER BY %s %s\n", filed, sort)
		sql = sql + order
	} else {
		sql = sql + "ORDER BY acc_transaction_count DESC\n"
	}

	err = tx.Raw(sql).
		Find(&transactions).Error
	if err != nil {
		return
	}
	count = len(transactions)

	offsetSize := fmt.Sprintf("OFFSET %d\n", page*limit)
	limitSize := fmt.Sprintf("LIMIT %d\n", limit)
	sql = sql + offsetSize + limitSize

	err = tx.Debug().Raw(sql).
		Find(&transactions).Error
	if err != nil {
		return
	}
	return
}

func (e EvmTransactionDal) GetEvmTransactionsAfterEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.EvmTransaction, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch >= ?", epoch.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (e EvmTransactionDal) GetEvmTransactions(ctx context.Context, epochs chain.LCRCRange) (items []*po.EvmTransaction, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch >= ? and epoch <= ?", epochs.GteBegin.Int64(), epochs.LteEnd.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (e EvmTransactionDal) SaveEvmTransactions(ctx context.Context, infos []*po.EvmTransaction) (err error) {
	err = e.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (e EvmTransactionDal) DeleteEvmTransactions(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.evm_transactions where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (e EvmTransactionDal) DeleteEvmTransactionsBeforeEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.evm_transactions where epoch < ?`, gteEpoch.Int64()).Error
	return
}

func (e EvmTransactionDal) GetEvmTransactionStats(ctx context.Context) (accTransaction []*bo.EVMTransactionStats, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	sql := `
SELECT et.actor_id                                                     AS actor_id,
       actor_address                                                   AS actor_address,
       fcs.contract_name                                               AS contract_name,
       count(*) + COALESCE(ets.acc_transaction_count, 0)               AS acc_transaction_count,
       sum(gas_cost) + COALESCE(ets.acc_gas_cost, 0)                   AS acc_gas_cost,
       COALESCE(itx.count, 0) + COALESCE(ets.acc_internal_tx_count, 0) AS acc_internal_trace
FROM fevm.evm_transactions et
         LEFT JOIN (SELECT actor_id, contract_name FROM fevm.contract_sols WHERE is_main_contract = true) fcs
                   ON et.actor_id = fcs.actor_id
         LEFT JOIN (SELECT (regexp_matches(trace_id, '^[0-9]*-[0-9]*'))[1] AS traces,
                           COUNT(*)                                        AS count
                    FROM fevm.evm_transactions
                    WHERE is_block = false
                    GROUP BY traces) itx ON et.trace_id = itx.traces
         LEFT JOIN (SELECT actor_id, acc_transaction_count, acc_internal_tx_count, acc_gas_cost
                    FROM fevm.evm_transaction_stats
                    WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transaction_stats)) ets ON et.actor_id = ets.actor_id
WHERE et.is_block = true
GROUP BY et.actor_id, actor_address, fcs.contract_name, itx.count, ets.acc_transaction_count, ets.acc_gas_cost, ets.acc_internal_tx_count;
`
	err = tx.Raw(sql).
		Find(&accTransaction).Error
	return
}

func (e EvmTransactionDal) SaveEvmTransactionStats(ctx context.Context, infos []*po.EvmTransactionStat) (err error) {
	err = e.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (e EvmTransactionDal) DeleteEvmTransactionStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.evm_transaction_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (e EvmTransactionDal) SaveEvmTransactionUser(ctx context.Context, infos *po.EvmTransactionUser) (err error) {
	err = e.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "actor_id"},
				{Name: "user_address"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"latest_tx_epoch"})}).
			CreateInBatches(infos, 100).Error
	})
	return
}
