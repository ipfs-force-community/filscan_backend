-- ops/wincount_gas_reward/03_validate.sql
--
-- 回填校验 / 巡检。全部按分区裁剪，别把 epoch 谓词去掉。
-- 用法：
--   /root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/03_validate.sql
-- 结论口径：**只要「NULL 行数」为 0，读路径就会走 PG；非 0 的区间读路径自动回落聚合器（不产生错值）。**

-- ============================================================
-- A. 全表回填进度（唯一需要看的「一眼结论」）
-- ============================================================
-- est_* 用 reltuples 估算（全表 1600 万行，不要 count(*)）。
-- 精确版（慢，按需在单分区上跑）见 C 段。
select (select coalesce(sum(c.reltuples), 0)::bigint
        from pg_class c
        where c.oid in (select inhrelid from pg_inherits where inhparent = 'chain.miner_win_counts'::regclass)) as est_rows,
       (select count(*) from pg_inherits where inhparent = 'chain.miner_win_counts'::regclass)           as partitions,
       (select count(*) from pg_attribute a
        where a.attrelid = 'chain.miner_win_counts'::regclass
          and a.attname = 'gas_reward' and a.attnum > 0 and not a.attisdropped)                          as has_gas_reward_col;
-- 期望：has_gas_reward_col = 1

-- ============================================================
-- B. 逐分区回填进度（精确，单分区扫 ~5 万行，秒级）
--    输出里 null_rows > 0 的分区就是 backfill.sh 还没覆盖到的。
-- ============================================================
select c.relname,
       count(*)                            as rows,
       count(gas_reward)                   as backfilled_rows,
       count(*) - count(gas_reward)         as null_rows,
       count(*) filter (where gas_reward < 0) as negative_rows
from pg_inherits i
         join pg_class c on c.oid = i.inhrelid
         join chain.miner_win_counts w on w.epoch >= (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'FROM ..([0-9]+).'))[1]::bigint
                                      and w.epoch < (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO ..([0-9]+).'))[1]::bigint
where i.inhparent = 'chain.miner_win_counts'::regclass
group by c.relname
order by c.relname desc;
-- 期望：negative_rows 恒为 0（**归一化之后**；归一化见 05_normalize_negatives.sql）。
--       回填刚跑完、还没归一化时，负值是**正常**的：2026-09-30 生产实测 2,015,596 行负值里
--         1,987,583 行（98.6%）恰好 = -1 ⇒ 只是 ±1 attoFIL 取整边界的下沿，真值必为 0（可证），
--                                     跑 05 精确置 0 即可，**不是模型失真**；
--            28,013 行（1.4%）≤ -2   ⇒ 这才是真偏差（penalty>0 / 奖励 actor 余额不足，
--                                     量级 ≤ ~1e-6 FIL），跑 05 置 NULL ⇒ 读路径整请求回落聚合器。
--       所以：先按「= -1 / ≤ -2」拆开看；只有 ≤ -2 的那批才需要人工核对聚合器返回值。

-- ============================================================
-- C. 单分区精确体检（把 <PARTITION> 换成 B 段输出的分区名）
-- ============================================================
-- select count(*)                              as rows,
--        count(gas_reward)                     as backfilled_rows,
--        count(*) - count(gas_reward)           as null_rows,
--        count(*) filter (where gas_reward < 0) as negative_rows,
--        count(*) filter (where win_count is null) as null_win_count_rows,
--        min(epoch), max(epoch)
-- from chain.miner_win_counts_<PARTITION>;

-- ============================================================
-- D. 抽样核对「本式回填 == 聚合器」的一致性（用 crosscheck.sh 打聚合器对比）
--    这里只给出要抽样的 (epoch, miner)：挑「gas_reward 非 0」的行最有信息量。
-- ============================================================
select epoch, miner, win_count, gas_reward
from chain.miner_win_counts
where epoch >= 6330000
  and epoch < 6330060
  and gas_reward is not null
  and gas_reward <> 0
order by epoch, miner
limit 10;
-- 然后把上面的 epoch 逐个喂给 crosscheck.sh：
--   ops/wincount_gas_reward/crosscheck.sh 6330000 6330060

-- ============================================================
-- E. 回填「够不够用」：按区间统计未回填高度（读路径会为这些区间回落聚合器）
-- ============================================================
-- epoch → 时间必须带主网创世时刻（2020-08-24 22:00:00 UTC），否则日期会算成 1976 年。
select date_trunc('week', timestamptz '2020-08-24 22:00:00+00' + make_interval(secs => epoch * 30)) as week,
       count(*) filter (where gas_reward is null) as null_rows,
       count(*)                                  as rows
from chain.miner_win_counts
where epoch >= 6300000
  and epoch < 6413000
group by 1
order by 1;

-- ============================================================
-- F. 幂等性自检：再跑一次 backfill.sh 应该是 0 行更新
-- ============================================================
-- 直接看 backfill.sh --dry-run 的输出即可；所有分区都应是 0。
