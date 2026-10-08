-- migration/37.reward_stream_recipient_epoch.sql
--
-- 「奖励流受益方按高度快照」表 —— 只服务「当前链上周期」，把 f02 奖励 actor 的服务流
-- （NV29 / FIP-0118，节点端点 POST /adapter/reward_stream_ledger）在每个高度留痕：
-- 每个（显式流）受益方一行，外加 tombstone（已移除但仍欠款）里的遗留受益方各一行。
--
-- ===== 为什么要有这张表（产品口径）=====
-- 链上只保存 f02 奖励流的**当前状态**：一个受益方被移出某条流、且欠款提完之后，
-- 它会从状态里彻底消失（实测：cali 的 t0199897 昨天在状态里、今天没了）。
-- 产品要求「本周期内出现过的受益方都要留痕」，从而能在页面上标出「本期已离场」——
-- 单看链头状态做不到（消失的人查不到），必须在每个高度把当时在场的受益方快照下来。
--
-- ===== 行从哪来（输入）=====
-- 同步器侧计算器 calc-reward-stream-recipient-task 调
-- ctx.Adapter().RewardStreamLedger(epoch) 取该高度账本，按下列规则落行：
--   1) 每条**显式流**（Implicit=false）的每个受益方：share / payable / claimed_period 取链上值；
--   2) ledger.tombstones 里的每个遗留受益方：share 记 0、payable 取链上值、claimed_period 记 0、
--      tombstone=true；
--   3) 节点返回 nv29=false（本网尚未激活 NV29）时**整高度不写**。
--
-- ===== 为什么是「每地址一行」而不是「每流一行」=====
-- 唯一键是 (epoch, address)：同一地址同时出现在多条显式流里时必须合并成一行
-- （否则 upsert 会互相覆盖、丢数据）。合并口径与展示层
-- modules/filscan/biz/browser/biz_statistic_reward_stream_ledger.go 的
-- buildRewardStreamRecipients 完全一致：share / payable / claimed_period 累加。
--
-- ===== 为什么需要 tombstone 列 =====
-- 不能用「share = 0」反推「已离场」：链上存在「流还在、份额被置 0」的**活跃**收款人
-- （2026-10-09 cali 实测 t0200442）。只有 tombstone=true 才表示该地址出现在已移除流里，
-- 展示层的 removed_stream 判据 = tombstone && share = 0。缺这一列会与 zero_share 混淆。
--
-- ===== 幂等性 / 重跑纪律 =====
-- 唯一键 (epoch, address) + Save 侧 ON CONFLICT (epoch, address) DO UPDATE：
-- 同一高度重跑（重试、回放）后表内容与只跑一次完全相同，不会产生重复行。
-- 重跑某高度前若想彻底清干净，可 delete from chain.reward_stream_recipient_epoch where epoch = <高度>;
--
-- ===== 为什么**不**分区（与 chain.miner_reward_stats 的关键差异）=====
-- chain schema 里按 epoch 分区的表（含 miner_reward_stats）靠一条 INSERT 规则把整行
-- 路由进分区（migration/36 已记载：`ON CONFLICT clause is not supported on tables with
-- INSERT rules`）。本表的核心语义是 upsert，分区 + 规则会直接让 ON CONFLICT 报错。
-- 故本表**不分区**，用普通唯一索引保证幂等（与 migration/35 的 chain.large_transfers 同为非分区表）。
--
-- ===== 部署注意 =====
-- 本表必须先于代码上线前建好：表缺失时计算器每个高度都会写失败。
-- 幂等：create ... if not exists / create index ... if not exists，可重复执行。
--
-- ===== 回滚 =====
--   drop table if exists chain.reward_stream_recipient_epoch;

create table if not exists chain.reward_stream_recipient_epoch
(
    epoch          bigint         not null,               -- 高度
    address        text           not null,               -- 受益方地址（robust 原文，如 f... / t...）
    share          numeric(38, 0) not null default 0,     -- 该高度该地址份额之和（Denom=1e18 定点；纯 tombstone 行为 0）
    payable        numeric(38, 0) not null default 0,     -- 跨周期结转欠款（attoFIL；显式流 + tombstone 累加）
    claimed_period numeric(38, 0) not null default 0,     -- 本期已提（attoFIL；tombstone 无此字段，记 0）
    tombstone      boolean        not null default false   -- 该地址在本高度出现在「已移除流」（ledger.tombstones）
);

-- 唯一键：幂等 upsert 的落点，也是「该高度有哪些受益方」的读路径。
create unique index if not exists reward_stream_recipient_epoch_epoch_address_uindex
    on chain.reward_stream_recipient_epoch (epoch, address);

-- (address, epoch)：供 API 端聚合「某地址在本周期出现过的高度」与「本周期出现过的人」。
create index if not exists reward_stream_recipient_epoch_address_epoch_index
    on chain.reward_stream_recipient_epoch (address, epoch);

comment on table chain.reward_stream_recipient_epoch is
    'f02 奖励流（NV29/FIP-0118）受益方按高度快照：每高度每受益地址一行；唯一键 (epoch,address) 幂等 upsert';
comment on column chain.reward_stream_recipient_epoch.share is
    '该高度该地址份额之和，Denom=1e18 定点；只出现在已移除流里的地址记 0';
comment on column chain.reward_stream_recipient_epoch.tombstone is
    '该地址在本高度出现在 ledger.tombstones（已移除流）；展示层 removed_stream = tombstone && share = 0';
