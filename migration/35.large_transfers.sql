-- 大额转账稀疏类预计算表：把「按高度区间扫冷库」换成「读一张万行级小表」。
--
-- 背景：聚合器端点 transfer_message_for_large_amount / count_of_largeamount_transfers 的过滤条件是
-- ExecTrace.MsgRct.ExitCode=0 且 FIL>=10000（10000 FIL = 1e22 attoFIL）。该条件命中面极小
-- （全链约 2 万行），但线上必须靠 FIL_1 索引扫冷库、再回表取文档 —— 在 HDD 冷库上单次数百毫秒
-- 到数百秒（实测最慢 347 秒）。改成读本表后，查询退化为一次按 epoch 的索引区间扫描。
--
-- 实测体量（2026-09，逐库抽取）：mongo03 7,684 行 / 2.78 MB、mongo08 10,254 行 / 3.68 MB；
-- 全链合计万行级、约 360 B/行 —— 即整类数据不到 10 MB。
--
-- 字段与聚合器返回逐列对齐（SignedCid/RootSignedCid 的兜底逻辑已按线上管道解析：
-- Cid 取 SignedCid 优先、RootCid 取 RootSignedCid 优先；from/to 保留 robust 地址原文，
-- value 保留 attoFIL 十进制字符串，禁止经浮点）。
--
-- 部署注意：本表是「一次性回填 + 由同步器增量维护」的形态。历史部分由一次性抽取脚本灌入
-- （必须在快盘窗口内执行，因为要扫冷库）；新高度由同步任务顺带写。表缺失时读取侧必须能回退到
-- 聚合器路径，不得直接 500。

create table if not exists chain.large_transfers
(
    epoch     bigint       not null, -- 高度
    cid       text         not null, -- 消息 cid（SignedCid 优先，否则 Cid）
    root_cid  text,                  -- 根消息 cid（RootSignedCid 优先，否则 RootCid）
    from_addr text,                  -- 发送方（robust 地址，原样保留）
    to_addr   text,                  -- 接收方（同上）
    value     numeric(38, 0),        -- 金额（attoFIL）
    method    text,                  -- 方法名（Msg.MethodName）
    depth     integer,               -- 调用深度
    primary key (epoch, cid)
);

create index if not exists large_transfers_epoch_index
    on chain.large_transfers (epoch);
