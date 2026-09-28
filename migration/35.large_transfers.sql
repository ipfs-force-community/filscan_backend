-- 大额转账稀疏类预计算表：把「按高度区间扫冷库」换成「读一张 20 万行级小表」。
--
-- 背景：聚合器端点 transfer_message_for_large_amount / count_of_largeamount_transfers 的过滤条件是
-- ExecTrace.MsgRct.ExitCode=0 且 FIL>=10000（10000 FIL = 1e22 attoFIL）。该条件命中面小，但线上必须
-- 靠 FIL_1 索引扫冷库、再回表取文档 —— 实测该端点 HTTP 请求 60 秒零字节（挂住），是现役最慢端点之一。
-- 改成读本表后，列表查询退化为一次按 epoch 的索引区间扫描。
--
-- 实测体量（2026-09-28 逐库抽取，12 个库全量）：合计 245,116 行、约 90 MB JSON（≈360 B/行），
-- 单库最大 mongo12hot 77,368 行、mongo11 73,820 行。
--
-- 字段与聚合器返回逐列对齐（SignedCid/RootSignedCid 的兜底按线上管道解析：Cid 取 SignedCid 优先、
-- RootCid 取 RootSignedCid 优先；from/to 保留 robust 地址原文；value 保留 attoFIL 十进制字符串，
-- 禁止经浮点）。
--
-- 【口径关键】本表**故意不建主键/唯一索引**，与线上重数完全一致：
--   1) 线上 count 管道是 $group{_id:0, Count:{$sum:1}} —— 数的是**命中 trace 的行数**，不是去重后的消息数；
--      线上列表管道没有 $group，一条 trace 行就是一行输出。
--   2) 线上 TotalCount = 各冷库在自己区间内的行数**求和**，而分片区间在边界上重叠 ——
--      实测 262 个 (epoch,cid) 同时存在于相邻两个库且内容逐字节相同（mongo10+mongo11 226、
--      mongo11+mongo12hot 27、mongo08+mongo09 9），这些行线上会被算两次。
--   3) 同一个库内同一 (epoch,cid) 也可能有多行：同高度的竞争区块各留一行 trace、root_cid 不同
--      （实测 24 组），线上同样都返回。
--   ⇒ 预计算表必须原样保留上述重数（否则会比线上少 0.1% 的行、分页整体位移），
--     故只建按 epoch 的普通索引供区间分页使用。
--
-- 部署注意：本表是「一次性回填 + 由同步器增量维护」的形态。历史部分由一次性抽取灌入；新高度由同步
-- 任务顺带写。表缺失时读取侧必须能回退到聚合器路径，不得直接 500。

create table if not exists chain.large_transfers
(
    epoch     bigint       not null, -- 高度
    cid       text         not null, -- 消息 cid（SignedCid 优先，否则 Cid）
    root_cid  text,                  -- 根消息 cid（RootSignedCid 优先，否则 RootCid）
    from_addr text,                  -- 发送方（robust 地址，原样保留）
    to_addr   text,                  -- 接收方（同上）
    value     numeric(38, 0),        -- 金额（attoFIL）
    method    text,                  -- 方法名（Msg.MethodName）
    depth     integer                -- 调用深度
);

-- 列表按 Epoch 倒序分页（管道 $sort:{Epoch:-1} + $skip/$limit），普通 btree 可反向扫。
create index if not exists large_transfers_epoch_index
    on chain.large_transfers (epoch);

-- 回收/重跑用：装载前若已有数据，先 truncate 而不是去重（重数是口径的一部分）。
