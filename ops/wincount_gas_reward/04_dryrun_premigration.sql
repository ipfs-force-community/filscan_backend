-- ops/wincount_gas_reward/04_dryrun_premigration.sql
--
-- **migration 之前** 的回填影响面估算（纯只读，不写）。
--
-- 为什么需要它：`02_backfill.sql` 的 WHERE 里有 `w.gas_reward is null`，
-- 而 gas_reward 列要 migration/36 才存在 ⇒ 在 DDL 之前直接跑 `backfill.sh --dry-run`
-- 会得到 `ERROR: column w.gas_reward does not exist`（实测 2026-09-30）。
--
-- 等价性证明（本式与 02_backfill.sql 的行数**恒等**）：
--   migration/36 的 `add column gas_reward numeric`（不带 default、允许 NULL）
--   ⇒ 加列瞬间**全部既有行**的 gas_reward 都是 NULL
--   ⇒ 02_backfill.sql 的 `w.gas_reward is null` 在「DDL 刚跑完、还没回填」这一刻恒真
--   ⇒ 把该谓词去掉后统计的行数 == 回填将更新的行数（逐行相等，不是上界估计）。
--
-- 与 02_backfill.sql 逐句对齐的只有三处差异：
--   1) 去掉 `w.gas_reward is null`（理由见上）；
--   2) `update ... set gas_reward = ...` 换成 `select count(*)`；
--   3) 加 `:lo` / `:hi` 由调用方代入（与 backfill.sh 同约定）。
--
-- 代价：本查询只扫**一个分区**（分区裁剪，~5 万行），不扫全表、不碰 mongo / 聚合器。
-- 用法：
--   /root/ltr_pg.sh -At -F'|' -f 04_dryrun_premigration.sql   # 需先 sed 代入 :lo/:hi

with ter as (select epoch
             from chain.builtin_actor_states
             where actor = 'f02'
               and epoch >= :lo
               and epoch < :hi
               and state ->> 'ThisEpochReward' is not null),
     r as (select distinct on (epoch, miner) epoch, miner
           from chain.miner_rewards
           where epoch >= :lo
             and epoch < :hi
           order by epoch, miner, block_time desc nulls last)
select count(*)
from chain.miner_win_counts w
         join r on r.epoch = w.epoch and r.miner = w.miner
         join ter on ter.epoch = r.epoch
where w.win_count is not null
  and w.epoch >= :lo
  and w.epoch < :hi;
