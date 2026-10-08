-- migration/38.reward_stream_recipient_period.sql
--
-- 「奖励流受益方按周期归集」表 —— 服务「累计已收」这一产品口径，把链上 f02 奖励 actor
-- （NV29 / FIP-0118，节点端点 POST /adapter/reward_stream_ledger）里每个显式流受益方的
-- claimed_period，按**周期**归集成一行 (address, period_start_epoch)。
--
-- ===== 目标表 / 这张表存在的理由（产品口径）=====
-- 产品要展示每个受益方的「累计已收」。链上只保存**当前周期**的 ClaimedPeriod，到下一个周期
-- 就归零 —— 累计值链上根本不存在，只能由我们跨周期累加。
-- 按高度快照表 chain.reward_stream_recipient_epoch（migration/37）不能担当这个职责：
--   (a) 它会被同步框架的 HistoryClear 按高度**修剪**，只服务当前周期，跨周期的历史行会被删掉；
--   (b) 它按高度存的是「当时快照值」，同一个周期内的 claimed_period 会随高度重复出现，
--       直接 SUM 会把同一笔重复计入。
-- 所以另立本表：**按周期归集、只增不减、跨周期保留**，是「累计已收」的唯一来源
-- （上层按 address 取该地址全部周期行再 SUM 即得累计）。
--
-- ===== 行从哪来（输入）=====
-- 同步器侧计算器 calc-reward-stream-recipient-task 在其 Calc 里，用**同一份 ledger**
-- （绝不额外请求节点）算出：
--   periodStart = nv29Epoch + ((epoch - nv29Epoch) / periodLen) * periodLen     （整除，向下取整）
-- 其中 nv29Epoch = message_detail.UpgradeSolsticeHeight（本网 NV29 激活高度，主网当前未排期），
--      periodLen = buildconstants.SolsticeEpochsPerQuarter（主网 262974；calibnet build tag 下为 EpochsInDay=2880）。
-- 对每条**显式流**（Implicit=false）的每个受益方，把该高度链上 claimed_period 累加到
-- (address, periodStart)。ledger.tombstones 的遗留受益方 claimed_period 记 0，**不进本表**。
-- 节点返回 nv29=false（本网尚未激活 NV29）时整高度不写（与快照表一致）。
--
-- ===== 值 / 高度与链上字段的对应 =====
--   claimed_in_period : 该周期内链上 claimed_period 的**最大观测值**（attoFIL）。
--     链上周期内 claimed_period 单调不减，故「周期内最大值」即「该周期最终已提」。
--     合并写取 GREATEST 正是为了让重复观测/重跑不把大值降回小值。
--   last_share        : 该周期内**最近一次**观测到的份额之和（Denom=1e18 定点）。
--     供展示层给「本周期离场者」补 last_share_pct（离场后链上已查不到该地址的当前份额，
--     只有归集表还留着它最后一次的份额）。注意它是「按地址跨显式流累加的份额之和」，
--     与快照表 share 同口径（分母同为 Denom，折算用同一 sharePercent）。
--     合并写只在观测高度**更新**时替换（见下），不无脑覆盖。
--   period_start_epoch: 周期标识（周期起点高度），不是「本行最后一个高度」。
--   first_epoch / last_epoch: 本周期内首次 / 最近一次观测到该受益方的高度（诊断与回滚边界用）。
--
-- ===== 幂等性 / 重跑纪律 =====
-- 唯一键 (address, period_start_epoch) + Upsert 侧 ON CONFLICT (address, period_start_epoch) DO UPDATE：
--   claimed_in_period = GREATEST(已存, excluded)
--   first_epoch       = LEAST(已存, excluded)
--   last_epoch        = GREATEST(已存, excluded)
--   last_share        = 取「观测高度更大」的那一行的值：
--                       CASE WHEN excluded.last_epoch >= 已存.last_epoch THEN excluded.last_share
--                            ELSE 已存.last_share END
--                       （同高度重跑 excluded.last_epoch == 已存.last_epoch ⇒ 取新值，同日同值；
--                        新观测更旧 ⇒ 保留已存值，绝不用陈旧份额盖掉更新的）
-- 同一高度重跑（重试、回放）、同一周期跨高度重复观测后，表内容与只跑一次完全相同，
-- 且**绝不会把已记录的较大 claimed_in_period 覆盖成较小值**（这是与快照表覆盖式 upsert 的关键差异）。
--
-- ===== 为什么**不**清理（与 fast表/快照表的关键差异）=====
-- 同步框架要求实现 HistoryClear，但本表**不清理**：累计必须跨周期保留。
-- 删掉任一已闭合周期的行就永久丢失该周期已收（链上已归零，无法重建）。
-- 计算器的 HistoryClear 只记 debug 日志、不删任何行（理由见 calc-reward-stream-recipient-task.go）。
--
-- ===== 为什么**不**分区 =====
-- chain schema 里按 epoch 分区的表靠一条 INSERT 规则把整行路由进分区（migration/36 已记载：
-- `ON CONFLICT clause is not supported on tables with INSERT rules`）。本表的核心语义是 upsert，
-- 分区 + 规则会直接让 ON CONFLICT 报错。故本表**不分区**，用普通唯一索引保证幂等
-- （与 migration/35 / migration/37 同为非分区表）。
--
-- ===== 部署注意 =====
-- 本表必须先于代码上线前建好：表缺失时计算器每个高度都会写失败。
-- 幂等：create ... if not exists / create index ... if not exists，可重复执行。
--
-- ===== 回滚 =====
--   drop table if exists chain.reward_stream_recipient_period;

create table if not exists chain.reward_stream_recipient_period
(
    address            text           not null,               -- 受益方地址（robust 原文，如 f... / t...）
    period_start_epoch bigint         not null,               -- 周期起点高度（周期标识）
    claimed_in_period  numeric(38, 0) not null default 0,     -- 本周期已提（attoFIL）= 该周期内 claimed_period 最大观测值
    last_share         numeric(38, 0) not null default 0,     -- 该周期内最近一次观测到的份额之和（Denom=1e18 定点）
    first_epoch        bigint         not null,               -- 本周期内首次观测到该受益方的高度
    last_epoch         bigint         not null                -- 本周期内最近一次观测到该受益方的高度（回滚边界列）
);

-- last_share 为后补列：本表尚未上生产时是就地改（上面 create 已带该列）；为兼容「已跑过旧版 38 的库」，
-- 这里再补一条幂等 alter（already-exists 时无操作）。全新库由 create 建列，这里同样无操作。
alter table chain.reward_stream_recipient_period
    add column if not exists last_share numeric(38, 0) not null default 0;

-- 唯一键：幂等合并写的落点，也是「某地址的全部周期行」的读路径（前导列 address）。
create unique index if not exists reward_stream_recipient_period_address_period_uindex
    on chain.reward_stream_recipient_period (address, period_start_epoch);

-- (period_start_epoch)：唯一索引前导列是 address，按周期维度单查走不到它，另建索引。
create index if not exists reward_stream_recipient_period_period_start_index
    on chain.reward_stream_recipient_period (period_start_epoch);

comment on table chain.reward_stream_recipient_period is
    'f02 奖励流（NV29/FIP-0118）受益方按周期归集：每 (address,period_start_epoch) 一行；唯一键幂等合并写，只增不减、跨周期保留，是「累计已收」的唯一来源';
comment on column chain.reward_stream_recipient_period.period_start_epoch is
    '周期起点高度 = nv29Epoch + ((epoch-nv29Epoch)/periodLen)*periodLen；周期标识，非本行最后高度';
comment on column chain.reward_stream_recipient_period.claimed_in_period is
    '本周期已提（attoFIL）= 该周期内链上 claimed_period 的最大观测值；合并写取 GREATEST 防止重跑把大值降回小值';
comment on column chain.reward_stream_recipient_period.last_share is
    '该周期内最近一次观测到的份额之和（Denom=1e18 定点）；合并写按 last_epoch 取更新的那一行的值；供展示层给本周期离场者补 last_share_pct';
comment on column chain.reward_stream_recipient_period.first_epoch is
    '本周期内首次观测到该受益方的高度；合并写取 LEAST';
comment on column chain.reward_stream_recipient_period.last_epoch is
    '本周期内最近一次观测到该受益方的高度；合并写取 GREATEST；RollBack 删除边界列（last_epoch >= gteEpoch）';
