-- ops/wincount_gas_reward/01_preflight.sql
--
-- 上线前只读体检（全部走 catalog / 单分区小查询，不扫全表）。逐条看输出，任何一条不符预期就停。
-- 用法（线上 PG 用现成包装脚本）：
--   /root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/01_preflight.sql
-- 或逐条 -c 跑。

-- 1) 父表是分区表、分区数、当前是否已有 gas_reward 列
select c.relkind                                                             as relkind,
       (select count(*) from pg_inherits i where i.inhparent = c.oid)         as partitions,
       (select count(*)
        from pg_attribute a
        where a.attrelid = c.oid
          and a.attname = 'gas_reward'
          and a.attnum > 0
          and not a.attisdropped)                                            as has_gas_reward_col
from pg_class c
where c.oid = 'chain.miner_win_counts'::regclass;
-- 期望：relkind = p，partitions = 317（数量会随时间增长），迁移前 has_gas_reward_col = 0

-- 2) INSERT 规则还在（决定「不能用 ON CONFLICT」「必须级联加列」两条约束）
select c.relname, r.rulename, pg_get_ruledef(r.oid)
from pg_rewrite r
         join pg_class c on c.oid = r.ev_class
where c.relname in ('miner_win_counts', 'miner_rewards')
  and r.rulename = 'range_insert_action_rule';
-- 期望：两行，规则动作都是 SELECT chain.action_*_range_insert(new.*)

-- 3) 分区表按 epoch 的周分区边界（用于确认回填覆盖范围与「当前周分区是否已存在」）
select c.relname,
       (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'FROM ..([0-9]+).'))[1]::bigint as lo,
       (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO ..([0-9]+).'))[1]::bigint   as hi
from pg_inherits i
         join pg_class c on c.oid = i.inhrelid
where i.inhparent = 'chain.miner_win_counts'::regclass
order by hi desc
limit 3;
-- 期望：最新分区 hi 覆盖链头；若中间缺周（同步空洞），这里会看到 hi 跳号

-- 4) 行数（估计值，不要 count(*)）与分区数
select (select coalesce(sum(c.reltuples), 0)::bigint
        from pg_class c
        where c.oid in (select inhrelid from pg_inherits where inhparent = 'chain.miner_win_counts'::regclass)) as est_rows;
-- 期望：~1600 万（2026-09-30 实测 16,046,889）

-- 5) 回填数据源是否在位：奖励 actor 状态表里有 f02（ThisEpochReward 的来源）
select count(*) as builtin_actor_states_partitions
from pg_class c
         join pg_namespace n on n.oid = c.relnamespace
where n.nspname = 'chain'
  and c.relname like 'builtin_actor_states%'
  and c.relkind = 'r';
-- 期望：> 0；且下面这条能取到值（挑一个 miner_win_counts 有数据的高度）
select epoch, (state ->> 'ThisEpochReward')::numeric as this_epoch_reward
from chain.builtin_actor_states
where actor = 'f02'
  and epoch = 6330000;
-- 期望：20939809989941853428（2026-09-30 实测）

-- 6) 覆盖对齐：抽样高度上 miner_win_counts 与 f02 状态是否同时有数据
--    （回填公式要求两者都在；只有一边有数据的 epoch 会留在 NULL，由读路径回落聚合器兜住）
with e(epoch) as (values (2160), (100000), (1000000), (2000000), (3000000),
                         (4000000), (5000000), (5800000), (6330000), (6412000))
select e.epoch,
       (select count(*) from chain.miner_win_counts w where w.epoch = e.epoch)                  as win_count_rows,
       (select count(*) from chain.builtin_actor_states s where s.epoch = e.epoch and s.actor = 'f02') as ter_rows
from e
order by e.epoch;
-- 2026-09-30 实测：2160→61/1；100000/1000000/2000000/3000000→0/0（该区间本来就没有 win_counts 行）；
--                4000000→5/1；5000000→6/1；5800000→9/1；6330000→11/1；6412000→1/1
