package po

// LargeTransfer 大额转账预计算表 chain.large_transfers 的一行（DDL 见 migration/35.large_transfers.sql）。
//
// 口径与线上聚合器管道 transfer_message_for_large_amount.js 逐列对齐（见
// londobell-aggregators/pool-monitor/transfer_message_for_large_amount.js 的 $project）：
//   - Epoch    ← ExecTrace.Epoch
//   - Cid      ← SignedCid 优先，否则 Cid（空串也回退，语义见任务里的 cidOf）
//   - RootCid  ← RootSignedCid 优先，否则 RootCid；**根消息（IsBlock/Depth=1）本身没有 root**，
//     线上该两字段未被写入（londobell racailum/segment/model/exec_trace.go genRootids 对 IsBlock 直接 return），
//     聚合器投影因此得到 null ⇒ 这里写成 NULL（指针 nil），与一次性回填的历史行一致。
//   - FromAddr / ToAddr ← Msg.From / Msg.To 的**原文**：mongo 侧编码为 addr.String()[1:]
//     （去掉网络前缀，londobell racailum/segment/model/codec.go addressBSONEncode），
//     写入时按任务里的 crudeAddress() 还原同一形态（兼容「已带前缀」与「已是原文」两种输入，
//     直接用 SmartAddress.CrudeAddress() 会把已无前缀的地址变成空串）。
//   - Value    ← Msg.Value，attoFIL 十进制字符串（mongo 存的就是 big.Int 的十进制文本，
//     bigIntBSONEncode）；**全程不经过浮点**，写入时按字符串绑定到 numeric(38,0)。
//   - Method   ← Msg.MethodName
//   - Depth    ← Depth
//
// 注意：线上口径要求保留 trace 行重数（同一高度同一 cid 可能多行），本表**没有主键、也没有唯一索引**，
// 因此不能用 gorm 的 upsert（on conflict）；维护动作是「同一事务内先按 epoch 删、再批量插」，
// 见 repository.LargeTransferRepo / dal.LargeTransferDal。
type LargeTransfer struct {
	Epoch    int64   // 高度
	Cid      string  // 消息 cid（SignedCid 优先，否则 Cid）
	RootCid  *string // 根消息 cid（RootSignedCid 优先，否则 RootCid）；根消息自身为 NULL
	FromAddr string  // 发送方（robust 地址原文，无网络前缀）
	ToAddr   string  // 接收方（同上）
	Value    string  // 金额（attoFIL 十进制字符串）
	Method   string  // 方法名
	Depth    int     // 调用深度
}

func (LargeTransfer) TableName() string {
	return "chain.large_transfers"
}
