package dal

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewLargeTransferDal(db *gorm.DB) *LargeTransferDal {
	return &LargeTransferDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.LargeTransferRepo = (*LargeTransferDal)(nil)

type LargeTransferDal struct {
	*_dal.BaseDal
}

// largeTransferInsertBatch 单次 INSERT 的行数上限。命中面极小（全链万行级、单高度通常 0~数行），
// 这里只是防御性上限，不是为了吞吐。
const largeTransferInsertBatch = 200

// ReplaceLargeTransfers 用 items 整体替换该高度已有的行：**同一个事务**里先 delete 再批量 insert。
//
// 为什么不 upsert：线上口径要求保留 trace 行重数（同一高度同一 cid 可能多行），
// 本表**没有主键、也不能加唯一索引**（见 migration/35.large_transfers.sql），
// 因此没有可用的冲突键，只能整体替换。
//
// 为什么必须同一事务：否则崩溃/中断会留下「已删未插」的半截状态或重复行；
// 同一事务保证「要么是旧内容、要么是新内容」，同一高度重复执行的结果与只执行一次完全相同。
//
// 为什么 Value 按字符串传：列是 numeric(38,0)，值是 attoFIL 十进制原文（mongo 里就是 big.Int 的
// 十进制文本，londobell bigIntBSONEncode）。绑定 Go string 由 PG 按 numeric 参数推断类型转换，
// 全程不经过浮点（shopspring decimal.Value() 返回的也是字符串，二者等价）。
func (d LargeTransferDal) ReplaceLargeTransfers(ctx context.Context, epoch int64, items []*po.LargeTransfer) (err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return
	}

	return tx.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec(`delete from chain.large_transfers where epoch = ?`, epoch).Error; e != nil {
			return e
		}
		if len(items) == 0 {
			return nil
		}
		return tx.CreateInBatches(items, largeTransferInsertBatch).Error
	})
}

// DeleteLargeTransfersFromEpoch 删除 >= gteEpoch 的行（链回滚路径）。
// 单条 delete、不涉及插入，失败时由同步器侧按「不阻断管线」的约定处理（见任务的 RollBack）。
func (d LargeTransferDal) DeleteLargeTransfersFromEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return
	}
	return tx.Exec(`delete from chain.large_transfers where epoch >= ?`, gteEpoch.Int64()).Error
}
