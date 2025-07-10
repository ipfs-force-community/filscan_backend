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
)

func NewEVMTransferDal(db *gorm.DB) *EVMTransferDal {
	return &EVMTransferDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.EvmTransferRepo = (*EVMTransferDal)(nil)

type EVMTransferDal struct {
	*_dal.BaseDal
}

func (e EVMTransferDal) GetTxsOfContractsByRange(ctx context.Context, start, end chain.Epoch) ([]*po.EvmTransfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := []*po.EvmTransfer{}
	err = tx.Model(&po.EvmTransfer{}).Select("epoch").Find(&res, "epoch >= ? and epoch < ?", start, end).Error
	return res, err
}

func (e EVMTransferDal) CountVerifiedContracts(ctx context.Context) (int64, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, err
	}
	res := int64(0)
	err = tx.Model(&po.FEvmContracts{}).Count(&res).Error
	return res, err
}

func (e EVMTransferDal) CountUniqueContracts(ctx context.Context, epoch chain.Epoch) (int64, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, err
	}

	res := int64(0)
	err = tx.Model(&po.EvmTransfer{}).Where("epoch <= ?", epoch).Distinct("actor_address").Count(&res).Error
	return res, err
}

func (e EVMTransferDal) CountTxsOfContracts(ctx context.Context, epoch chain.Epoch) (int64, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, err
	}

	res := int64(0)
	err = tx.Model(&po.EvmTransfer{}).Where("epoch <= ?", epoch).Count(&res).Error
	return res, err
}

func (e EVMTransferDal) CountUniqueUsers(ctx context.Context, epoch chain.Epoch) (int64, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, err
	}

	res := int64(0)
	err = tx.Model(&po.EvmTransfer{}).Where("epoch <= ?", epoch).Distinct("user_address").Count(&res).Error
	return res, err
}

func (e EVMTransferDal) GetEvmTransferStatsByID(ctx context.Context, actorID string) (evmTransfer *po.EvmTransferStat, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	evmTransfer = new(po.EvmTransferStat)
	sql := `
SELECT *
FROM fevm.evm_transfer_stats
WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transfer_stats WHERE interval = '1h') AND actor_id = ?
`
	err = tx.Raw(sql, actorID).First(evmTransfer).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			evmTransfer = nil
			err = nil
		}
		return
	}
	return
}

func (e EVMTransferDal) GetEvmTransferStatsByContractName(ctx context.Context, contractName string) (evmTransfer *po.EvmTransferStat, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	evmTransfer = new(po.EvmTransferStat)
	sql := `
SELECT *
FROM fevm.evm_transfer_stats
WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transfer_stats WHERE interval = '1h') AND contract_name = ?
`
	err = tx.Raw(sql, contractName).First(evmTransfer).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			evmTransfer = nil
			err = nil
		}
		return
	}
	return
}

func (e EVMTransferDal) GetEvmTransferByID(ctx context.Context, actorID string) (evmTransfer *bo.EVMTransferStatsWithName, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT et.actor_id                    AS actor_id,
	   fcs.contract_name              AS contract_name,
       count(*)                       AS acc_transfer_count,
       count(DISTINCT (user_address)) AS acc_user_count,
       sum(gas_cost)                  AS acc_gas_cost
FROM fevm.evm_transfers et
LEFT JOIN (SELECT actor_id, contract_name FROM fevm.contract_sols WHERE is_main_contract = true) fcs
                   ON et.actor_id = fcs.actor_id
WHERE et.actor_id = ?
GROUP BY et.actor_id, fcs.contract_name
`
	err = tx.Raw(sql, actorID).Find(&evmTransfer).Error
	if err != nil {
		return
	}
	return
}

func (e EVMTransferDal) GetEvmTransferStatsList(ctx context.Context, page, limit int, filed, sort, interval string) (transfers []*po.EvmTransferStat, count int, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT *
FROM fevm.evm_transfer_stats
WHERE epoch = (SELECT max(epoch) FROM fevm.evm_transfer_stats WHERE interval = '1h')
`
	if filed != "" && sort != "" {
		if filed == "transfer_count" {
			filed = "acc_transfer_count"
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
		sql = sql + "ORDER BY acc_transfer_count DESC\n"
	}

	err = tx.Raw(sql).
		Find(&transfers).Error
	if err != nil {
		return
	}
	count = len(transfers)

	offsetSize := fmt.Sprintf("OFFSET %d\n", page*limit)
	limitSize := fmt.Sprintf("LIMIT %d\n", limit)
	sql = sql + offsetSize + limitSize

	err = tx.Debug().Raw(sql).
		Find(&transfers).Error
	if err != nil {
		return
	}
	return
}

func (e EVMTransferDal) GetEvmTransferList(ctx context.Context, epochs *chain.LORCRange, page, limit int, filed, sort, interval string) (transfers []*bo.EvmTransfers, count int, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	//sql := `
	//SELECT et.actor_id                    AS actor_id,
	//    actor_address                  AS actor_address,
	//    fcs.contract_name              AS contract_name,
	//    count(*)                       AS transfer_count,
	//    count(DISTINCT (user_address)) AS user_count,
	//    sum(gas_cost)                  AS gas_cost,
	//    et1.balance                    AS actor_balance
	//FROM fevm.evm_transfers et
	//      LEFT JOIN (SELECT actor_id, contract_name FROM fevm.contract_sols WHERE is_main_contract = true) fcs
	//                ON et.actor_id = fcs.actor_id
	//      LEFT JOIN (SELECT t.actor_id, t.balance, t.epoch
	//                 FROM (SELECT actor_id,
	//                              balance,
	//                              epoch,
	//                              row_number() over (PARTITION BY actor_id ORDER BY epoch DESC) AS rn
	//                       FROM fevm.evm_transfers) t
	//                 WHERE t.rn = 1) et1 ON et.actor_id = et1.actor_id
	//GROUP BY et.actor_id, actor_address, fcs.contract_name, et1.balance
	//`
	sql := `
SELECT et.actor_id                                                               AS actor_id,
       et.actor_address                                                          AS actor_address,
       et.balance                                                                AS actor_balance,
       et.contract_name                                                          AS contract_name,
       COALESCE(aet.acc_transfer_count, 0) + COALESCE(ets.acc_transfer_count, 0) AS transfer_count,
       COALESCE(aet.acc_user_count, 0) + COALESCE(ets.acc_user_count, 0)         AS user_count,
       COALESCE(aet.acc_gas_cost, 0) + COALESCE(ets.acc_gas_cost, 0)             AS gas_cost
FROM (SELECT t.actor_id, t.actor_address, t.balance, fcs.contract_name
      FROM (SELECT actor_id,
                   actor_address,
                   balance,
                   row_number() over (PARTITION BY actor_id ORDER BY epoch DESC) AS rn
            FROM fevm.evm_transfers) t
               LEFT JOIN (SELECT actor_id, contract_name FROM fevm.contract_sols WHERE is_main_contract = true) fcs
                         ON t.actor_id = fcs.actor_id
      WHERE t.rn = 1) et
         LEFT JOIN (SELECT actor_id,
                           sum(acc_transfer_count) AS acc_transfer_count,
                           sum(acc_user_count)     AS acc_user_count,
                           sum(acc_gas_cost)       AS acc_gas_cost
                    from fevm.evm_transfer_stats
                    where interval = '24h'
                    group by actor_id) ets ON et.actor_id = ets.actor_id
         LEFT JOIN (SELECT actor_id,
                           count(*)                       AS acc_transfer_count,
                           count(DISTINCT (user_address)) AS acc_user_count,
                           sum(gas_cost)                  AS acc_gas_cost
                    FROM fevm.evm_transfers
                    WHERE epoch > ?
                    GROUP BY actor_id) aet ON et.actor_id = aet.actor_id
`
	if filed != "" && sort != "" {
		order := fmt.Sprintf("ORDER BY %s %s\n", filed, sort)
		sql = sql + order
	} else {
		sql = sql + "ORDER BY transfer_count DESC\n"
	}

	err = tx.Raw(sql, chain.CurrentEpoch().CurrentDay()).
		Find(&transfers).Error
	if err != nil {
		return
	}
	count = len(transfers)

	offsetSize := fmt.Sprintf("OFFSET %d\n", page*limit)
	limitSize := fmt.Sprintf("LIMIT %d\n", limit)
	sql = sql + offsetSize + limitSize

	err = tx.Raw(sql, chain.CurrentEpoch().CurrentDay()).
		Find(&transfers).Error
	if err != nil {
		return
	}
	return
}

func (e EVMTransferDal) SaveEvmTransfers(ctx context.Context, infos []*po.EvmTransfer) (err error) {
	err = e.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (e EVMTransferDal) DeleteEvmTransfers(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.evm_transfers where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (e EVMTransferDal) GetEvmTransferStats(ctx context.Context, epoch chain.Epoch) (accTransfer []*bo.EVMTransferStats, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	sql := `
SELECT et.actor_id                    AS actor_id,
       actor_address                  AS actor_address,
       fcs.contract_name              AS contract_name,
       count(*)                       AS acc_transfer_count,
       count(DISTINCT (user_address)) AS acc_user_count,
       sum(gas_cost)                  AS acc_gas_cost,
       et1.balance                    AS actor_balance
FROM fevm.evm_transfers et
         LEFT JOIN (SELECT actor_id, contract_name FROM fevm.contract_sols WHERE is_main_contract = true) fcs
                   ON et.actor_id = fcs.actor_id
         LEFT JOIN (SELECT t.actor_id, t.balance, t.epoch
                    FROM (SELECT actor_id,
                                 balance,
                                 epoch,
                                 row_number() over (PARTITION BY actor_id ORDER BY epoch DESC) AS rn
                          FROM fevm.evm_transfers) t
                    WHERE t.rn = 1) et1 ON et.actor_id = et1.actor_id
WHERE et.epoch <= ?
GROUP BY et.actor_id, actor_address, fcs.contract_name, et1.balance
`
	err = tx.Raw(sql, epoch).
		Find(&accTransfer).Error
	return
}

func (e EVMTransferDal) SaveEvmTransferStats(ctx context.Context, infos []*po.EvmTransferStat) (err error) {
	err = e.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (e EVMTransferDal) DeleteEvmTransferStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from fevm.evm_transfer_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}
