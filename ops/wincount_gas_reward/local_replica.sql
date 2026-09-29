-- ops/wincount_gas_reward/local_replica.sql
--
-- **只用于本地/一次性库**：复刻线上 chain.miner_win_counts 的
-- 「RANGE 分区父表 + range_insert_action_rule + `INSERT INTO <part> SELECT $1.*`」形态，
-- 用来端到端验证 migration/36 与 ops/wincount_gas_reward/ 的脚本（不依赖生产、不依赖 mongo）。
--
-- 跑法（不要在生产上跑！库名必须含 test/probe）：
--   initdb -D /tmp/pgtest -U postgres --auth=trust
--   LC_ALL=C pg_ctl -D /tmp/pgtest -o "-p 55432 -k /tmp/pgsock -c listen_addresses=''" start
--   psql -h /tmp/pgsock -p 55432 -U postgres -c 'create database filscan_probe'
--   psql -h /tmp/pgsock -p 55432 -U postgres -d filscan_probe -f ops/wincount_gas_reward/local_replica.sql
--   psql -h /tmp/pgsock -p 55432 -U postgres -d filscan_probe -f migration/36.miner_win_counts_gas_reward.sql
--   PSQL_CMD="psql -h /tmp/pgsock -p 55432 -U postgres -d filscan_probe" \
--     ops/wincount_gas_reward/backfill.sh --dry-run
--   PSQL_CMD="psql -h /tmp/pgsock -p 55432 -U postgres -d filscan_probe" \
--     ops/wincount_gas_reward/backfill.sh --yes
--   psql -h /tmp/pgsock -p 55432 -U postgres -d filscan_probe -f ops/wincount_gas_reward/03_validate.sql
--
-- 期望（本文件 + 36 号迁移 + 回填后的实测结果）：
--   * 36 号迁移的两条后检：分区缺列数 = 0、列序号无不一致；
--   * 通过规则 INSERT 4 列正常落位（`SELECT $1.*` 仍对得上）；
--   * 回填后：6296700 的 4 个矿工 = {89145023322864, 214208455099239, 89145023322864, 0}；
--     6316900 = 777；6310000 保持 NULL（该高度没有 f02 状态）；6310001 = 负值（负值探针）；
--   * 二次 backfill.sh = 0 行（幂等）；`negative_rows` = 1。
--
-- 与线上的唯一差异：周分区的 epoch 换算被简化成 epoch/20160（线上是 epoch↔时间戳换算）。
create schema if not exists chain;

-- ============ 复刻线上表结构（migration/1.chain.sql:190 / :147 / :39）============
drop table if exists chain.miner_win_counts cascade;
create table chain.miner_win_counts
(
    epoch     bigint,
    miner     varchar,
    win_count bigint
) partition by range (epoch);
create index miner_win_counts_epoch_miner_index on only chain.miner_win_counts using btree (epoch, miner);
create index miner_win_counts_miner_epoch_index on only chain.miner_win_counts using btree (miner, epoch);

drop table if exists chain.miner_rewards cascade;
create table chain.miner_rewards
(
    epoch       bigint,
    miner       varchar,
    reward      numeric,
    block_count bigint,
    block_time  timestamp
) partition by range (epoch);
create unique index miner_rewards_epoch_miner_uindex on chain.miner_rewards using btree (epoch, miner);

drop table if exists chain.builtin_actor_states cascade;
create table chain.builtin_actor_states
(
    epoch   bigint,
    actor   varchar,
    state   jsonb,
    balance numeric
) partition by range (epoch);
create unique index builtin_actor_states_epoch_actor_uindex on chain.builtin_actor_states using btree (epoch, actor);

-- ============ 复刻线上 INSERT 规则的动作函数（线上：chain.action_miner_win_counts_range_insert）============
-- 关键点与线上一致：① 分区缺失就现建；② `INSERT INTO <part> SELECT $1.*` 按复合类型整行展开
-- ⇒ 加列后规则无需改动，但列必须级联到每个分区（否则列数不匹配、写入路径全挂）。
create or replace function chain.action_miner_win_counts_range_insert(table_row chain.miner_win_counts)
    returns void
    language plpgsql
as
$$
declare
    lo   bigint;
    hi   bigint;
    part text;
begin
    lo := (table_row.epoch / 20160) * 20160;
    hi := lo + 20160;
    part := format('miner_win_counts_w%s_%s_%s', lo / 20160, lo, hi);
    if not exists (select 1
                   from pg_class c
                            join pg_namespace n on n.oid = c.relnamespace
                   where n.nspname = 'chain'
                     and c.relname = part) then
        execute format('create table chain.%I partition of chain.miner_win_counts for values from (%s) to (%s)', part, lo, hi);
    end if;
    execute format('insert into chain.%I select $1.*', part) using table_row;
end
$$;

create or replace rule range_insert_action_rule as on insert to chain.miner_win_counts
    do instead select chain.action_miner_win_counts_range_insert(new.*);

-- miner_rewards / builtin_actor_states 在线上也有 INSERT 规则；本副本预建分区即可
create table if not exists chain.miner_rewards_w312 partition of chain.miner_rewards for values from (6289920) to (6310080);
create table if not exists chain.miner_rewards_w313 partition of chain.miner_rewards for values from (6310080) to (6330240);
create table if not exists chain.builtin_actor_states_w312 partition of chain.builtin_actor_states for values from (6289920) to (6310080);
create table if not exists chain.builtin_actor_states_w313 partition of chain.builtin_actor_states for values from (6310080) to (6330240);

-- ============ 造数据 ============
-- 两个周分区 w312 / w313。每个 epoch 4 个矿工；按 blockReward = trunc(T*wc/5) 造
-- reward = blockReward + gas_reward（模拟 specs-actors 的 totalReward = blockReward + GasReward）。
do
$$
    declare
        v_e      bigint;
        i        int;
        v_ter    numeric := 20939809989941853428;
        v_wc     bigint;
        v_gas    numeric;
        v_blk    numeric;
        v_miners text[] := array ['f01083914', 'f01859603', 'f01938674', 'f02825420'];
    begin
        for v_e in 6296700..6296705 loop
            insert into chain.builtin_actor_states (epoch, actor, state)
            values (v_e, 'f02', jsonb_build_object('ThisEpochReward', v_ter::text));
            for i in 1..4 loop
                v_wc := 1 + (i % 2);                                   -- 1 或 2
                v_blk := trunc(v_ter * v_wc / 5);
                -- 第 2 个矿工有 gas，第 4 个矿工 gas 合法为 0
                v_gas := case when i = 2 then 214208455099239 when i = 4 then 0 else 89145023322864 end;
                -- miner_rewards 有唯一索引 (epoch,miner)（线上同），只插一份
                insert into chain.miner_rewards (epoch, miner, reward, block_count, block_time)
                values (v_e, v_miners[i], v_blk + v_gas, v_wc, '2026-09-01 00:00:00');
                -- win_counts 插 2 份（同一 (epoch,miner) 重复行，读路径必须去重）
                insert into chain.miner_win_counts (epoch, miner, win_count)
                values (v_e, v_miners[i], v_wc),
                       (v_e, v_miners[i], v_wc);
            end loop;
        end loop;
    end
$$;

-- 造一个「有 win_counts 但缺 f02 状态」的 epoch（回填公式取不到 ThisEpochReward ⇒ 应留在 NULL，
-- 读路径据此回落聚合器）
insert into chain.miner_rewards (epoch, miner, reward, block_count, block_time)
values (6310000, 'f01083914', 4139128251386364483, 1, '2026-09-03 00:00:00');
insert into chain.miner_win_counts (epoch, miner, win_count)
values (6310000, 'f01083914', 1);

-- w313 里的一个 epoch（跨分区回填）
insert into chain.builtin_actor_states (epoch, actor, state)
values (6316900, 'f02', jsonb_build_object('ThisEpochReward', '20000000000000000000'));
insert into chain.miner_rewards (epoch, miner, reward, block_count, block_time)
values (6316900, 'f02825420', 4000000000000000000 + 777, 1, '2026-09-05 00:00:00');
insert into chain.miner_win_counts (epoch, miner, win_count)
values (6316900, 'f02825420', 1);

-- 造一个「gas_reward 会被算成负数」的 epoch：reward 远小于 blockReward
-- （模拟奖励 actor 余额不足 / penalty）⇒ 03_validate.sql 的 negative_rows 必须报出来
insert into chain.builtin_actor_states (epoch, actor, state)
values (6310001, 'f02', jsonb_build_object('ThisEpochReward', '20939809989941853428'));
insert into chain.miner_rewards (epoch, miner, reward, block_count, block_time)
values (6310001, 'f01083914', 1000, 1, '2026-09-04 00:00:00');
insert into chain.miner_win_counts (epoch, miner, win_count)
values (6310001, 'f01083914', 1);
