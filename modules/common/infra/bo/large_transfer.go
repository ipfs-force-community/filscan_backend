package bo

import "github.com/shopspring/decimal"

// LargeTransferRow chain.large_transfers 的一行（大额转账预计算表，DDL 见 migration/35.large_transfers.sql）。
//
// 字段与聚合器端点 transfer_message_for_largeAmount 的**管道投影**逐列对齐
// （管线原文：londobell-aggregators/pool-monitor/transfer_message_for_large_amount.js）：
//
//	$match : { "MsgRct.ExitCode": 0, "Epoch": {$gte,$lt}, "FIL": {$gte: 10000} } ← 本表只装命中行
//	$project:
//	    Cid     = SignedCid 非 null ? SignedCid : Cid
//	    RootCid = RootSignedCid 非 null ? RootSignedCid : RootCid
//	    Epoch / From = Msg.From / To = Msg.To / Value = Msg.Value
//	    Method  = Msg.MethodName / Depth = $Depth
//
// 注意投影里**没有** SignedCid / ExitCode / IsBlock 三个键：
//
//	SignedCid —— 属性被并进了 Cid（见上），故 Cid 列本身就是「优先序列的解析结果」；
//	ExitCode  —— $match 已经把 ExitCode=0 变成前提，投影不再输出；
//	IsBlock   —— 只存在于聚合器的 TransferMessage 结构里，filscan 侧 ActorMessages 无该字段。
//
// ⇒ 映射到 londobell.ActorMessages 时，Cid 取本行的 Cid、RootCid 取本行的 RootCid，
// SignedCid / ExitCode 保持零值（与聚合器**完全相同**，不是「缺字段」）。
//
// Value 是 attoFIL 十进制原文（PG numeric(38,0)），**绝不经过 float64**：
// 大额行量级在 1e22 attoFIL 以上，float64 只有 15~16 位有效数字，一旦过浮点就丢精度。
type LargeTransferRow struct {
	Epoch    int64           `gorm:"column:epoch"`
	Cid      string          `gorm:"column:cid"`
	RootCid  string          `gorm:"column:root_cid"`
	FromAddr string          `gorm:"column:from_addr"`
	ToAddr   string          `gorm:"column:to_addr"`
	Value    decimal.Decimal `gorm:"column:value"`
	Method   string          `gorm:"column:method"`
	Depth    int64           `gorm:"column:depth"`
}
