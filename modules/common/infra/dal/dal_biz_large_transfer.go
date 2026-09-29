package dal

import (
	"context"
	"math"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

// 本文件是大额转账列表端点（/aggregators/transfer_message_for_largeAmount）的 PG 读实现（只读；不动写路径）。
//
// 口径来自聚合器**管线原文**（londobell-aggregators/pool-monitor/transfer_message_for_large_amount.js）：
//
//	$match : { "MsgRct.ExitCode": 0, "Epoch": {$gte: ctx.StartEpoch, $lt: ctx.EndEpoch}, "FIL": {$gte: 10000} }
//	$sort  : { "Epoch": -1 }
//	$skip  : ctx.Skip      ← multi-query/query.go:296-300 的 skip = index * limit
//	$limit : ctx.Limit
//	$project: Cid = SignedCid||Cid、RootCid = RootSignedCid||RootCid、
//	          Epoch / From = Msg.From / To = Msg.To / Value = Msg.Value /
//	          Method = Msg.MethodName / Depth = $Depth
//
// 以及计数管线（count_of_largeamount_transfers.js，同一 $match + $group{$sum:1}）。
// 表侧只装命中行（$match 已由装载方完成），所以读侧只需分页与定序。
//
// 与聚合器路径的**已知差异**（不假装逐行一致，详见 agg_pg_large_amount.go 与 parity 工具的口径行）：
//
//  1. 同高度内行序：线上每个冷库只做 `$sort:{Epoch:-1}`，同一高度内的行序**未定义**，
//     多库合并后的顺序也未定义；PG 侧用 `order by epoch desc, cid asc` 定序。
//     ⇒ 同一高度的多行顺序可能不同；若分页窗口正好切在一个高度组中间，该页的行**集合**
//     也可能不同（这不是 bug，是线上本来就没有确定性）。定序在这里是「确定性优于线上」的取舍。
//     parity 工具把这类差异单列为 order_diff（默认不判失败）并给出数量。
//  2. TotalCount：线上是各冷库在自己区间内 $group{$sum:1} 后**求和**（分片区间在边界重叠，
//     实测 262 个 (epoch,cid) 同时存在于相邻两库且内容逐字节相同 ⇒ 被算两次），
//     PG 侧是 `count(*)` 全表。两者同为「命中 trace 的**行数**」口径（都不去重），
//     但来源不同：线上取决于各库区间与装载时点，PG 取决于表内容。
//
// ⚠ 绝不能改成 distinct / group by / 加唯一约束：线上是按行数计的，去重会让结果比线上少行、
// 且分页整体位移（migration/35.large_transfers.sql 的口径说明）。
//
// value 列是 numeric(38,0) 的 attoFIL 原文（大额行 ≥ 1e22），扫进 decimal.Decimal 后原样转字符串，
// **绝不经过 float64**（float64 只有 15~16 位有效数字，过一次浮点就丢精度）。

// SQLLargeTransfersPage 大额转账分页（按 epoch 倒序、同高度按 cid 定序）。
//
// 三处 coalesce 是「与聚合器形态对齐」而不是防错：
//
//	root_cid/from_addr/to_addr/method 在 mongo 侧可能是 null，$project 原样输出 null，
//	filscan 的 ActorMessages 是 string 字段 ⇒ json.Unmarshal 得到 ""（空串）；
//	depth 为 null 时得到 0，value 为 null 时 shopspring 的 UnmarshalJSON 直接跳过（= 0）。
//	⇒ 库侧用同义的空串/0 落到 Go 结构体，两条路径的零值形态一致。
const SQLLargeTransfersPage = `
select epoch,
       cid,
       coalesce(root_cid, '')  as root_cid,
       coalesce(from_addr, '') as from_addr,
       coalesce(to_addr, '')   as to_addr,
       coalesce(value, 0)      as value,
       coalesce(method, '')    as method,
       coalesce(depth, 0)      as depth
from chain.large_transfers
order by epoch desc, cid asc
offset ? limit ?`

// SQLCountLargeTransfers TotalCount = 全表 count(*)。
//
// 三条理由（口径已在父任务里裁定，改这条 SQL 等于改口径）：
//
//  1. 请求不带高度区间（聚合器客户端只传 index/limit）⇒ 读侧不得按区间过滤；
//  2. 线上 count 管道数的是**命中 trace 的行数**（$group{$sum:1} 不去重），列表管道也没有 $group
//     ⇒ 不能写 count(distinct ...)；
//  3. 分片区间在边界重叠、同库同高度也可能多行（竞争区块）⇒ 重数是口径的一部分。
const SQLCountLargeTransfers = `
select count(*) as cnt
from chain.large_transfers`

// LargeTransferPageWindow 把聚合器的 (index, limit) 折算成 SQL 的 (offset, limit)。
//
// 依据线上原文 multi-query/query.go:296-300：
//
//	if indexReq > 0 || limitReq > 0 { skip = indexReq * limitReq; limit = limitReq }
//
// ⇒ **index 是页码（0 起）**，不是 offset；index 与 limit 都为 0 时线上不设分页、取全量
// （其内部把 limit 置为 MaxInt64），这里同样用 math.MaxInt64 表达「不设上限」。
//
// 负数输入不做钳制：线上这种输入会让 mongo 的 $skip 直接报错（聚合器回 code!=0），
// PG 侧让 offset/limit 报错后回落聚合器，两条路径的错误形态一致（都是接口失败，不是静默出错数据）。
func LargeTransferPageWindow(index, limit int64) (offset, pageLimit int64) {
	if index <= 0 && limit <= 0 {
		return 0, math.MaxInt64
	}
	return index * limit, limit
}

func NewLargeTransferDal(db *gorm.DB) *LargeTransferDal {
	return &LargeTransferDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.LargeAmountTransfer = (*LargeTransferDal)(nil)

// LargeTransferDal 大额转账列表端点的 PG 读实现。
type LargeTransferDal struct {
	*_dal.BaseDal
}

func (m LargeTransferDal) LargeTransfersPage(ctx context.Context, offset, limit int64) (items []*bo.LargeTransferRow, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	// 只读查询，不开事务（与既有 PG 读路径一致）。
	err = tx.Raw(SQLLargeTransfersPage, offset, limit).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m LargeTransferDal) CountLargeTransfers(ctx context.Context) (total int64, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	var row struct {
		Cnt int64 `gorm:"column:cnt"`
	}
	if err = tx.Raw(SQLCountLargeTransfers).Scan(&row).Error; err != nil {
		return
	}
	return row.Cnt, nil
}

// ===== 以下为 parity 工具（cmd/agg-parity）与运维自查用的只读诊断 SQL，不参与 API 读路径 =====

// SQLTableRangeLargeTransfers 表的 epoch 覆盖范围与总行数 —— 上线前确认装载是否到链头。
const SQLTableRangeLargeTransfers = `
select coalesce(min(epoch), 0) as min_epoch,
       coalesce(max(epoch), 0) as max_epoch,
       count(*)                as cnt
from chain.large_transfers`

// SQLDuplicateLargeTransfers 同 (epoch, cid) 的多行组数与行数——**不是错误**，是口径的一部分
// （竞争区块 root_cid 不同 / 分片区间边界重叠）。用于解释「TotalCount 比 distinct 行数大」。
const SQLDuplicateLargeTransfers = `
select epoch, cid, count(*) as cnt
from chain.large_transfers
group by epoch, cid
having count(*) > 1
order by cnt desc, epoch asc
limit ?`

// SQLDistinctCountLargeTransfers 去重后的 (epoch, cid) 组数——只做对照，**不得**用于读路径。
const SQLDistinctCountLargeTransfers = `
select count(*) as cnt
from (select distinct epoch, cid from chain.large_transfers) t`
