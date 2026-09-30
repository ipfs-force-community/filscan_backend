-- ops/wincount_gas_reward/90_rollback.sql
--
-- 回滚 wincount 的 gas_reward 改造。**按逆序做**，每一步都可单独停。
--
-- 回滚顺序（不可颠倒）：
--   1) 先把配置开关关掉（miner_wincount_read_from_pg = false）并重启 filscan-api
--      —— 读路径回到聚合器，此时 PG 侧有没有 gas_reward 都无所谓。
--   2) 再把 filscan-syncer 回滚到**不再写 gas_reward 的旧版本**并重启
--      —— 关键：只要写路径还在写 gas_reward，就不能删列（下一次同步的 INSERT 会因列不存在而失败）。
--   3) 最后才执行本文件的 DDL。
--
-- 可选：只关读路径（第 1 步）而保留写入。这是**推荐**的中间态 ——
-- 列留着、新数据继续攒、读路径先回聚合器，出问题时不用重来。

-- ===== 步骤 3a：仅停用（不删列、不丢回填数据）=====
-- 把列改名为 gas_reward_disabled 即可让新代码/老代码都不再引用它，数据留档：
--   alter table chain.miner_win_counts rename column gas_reward to gas_reward_disabled;
--   -- 回滚这次改名：
--   alter table chain.miner_win_counts rename column gas_reward_disabled to gas_reward;

-- ===== 步骤 3b：彻底删除（会丢掉全部回填结果，重来需要重跑 backfill.sh）=====
-- ⚠ 确认第 2 步已完成（没有任何进程在写 gas_reward）再执行。
-- ⚠ 分区父表上 drop column 同样级联到全部分区；不要加 ONLY。
--
-- alter table chain.miner_win_counts drop column if exists gas_reward;

-- ===== 步骤 3c：删列后的确认（应为 0 个分区还带该列）=====
--
-- select count(*)
-- from pg_inherits i
--          join pg_class c on c.oid = i.inhrelid
--          join pg_attribute a on a.attrelid = c.oid and a.attname = 'gas_reward' and a.attnum > 0
-- where i.inhparent = 'chain.miner_win_counts'::regclass;

-- ===== 代码回滚 =====
-- 本次改造全部在分支 feat/wincount-gas-reward 上，未合入 main：
--   cd <worktree> && git log --oneline main..HEAD
-- 回滚代码 = 切回 main（或 revert 该分支的提交）后重新构建 filscan-api / filscan-syncer / filscan-agg-parity。
-- 注意 migration/36 与 ops/ 都是新增文件，不影响 main 上的运行。

-- ===== 回滚后必做 =====
--   * filscan-api：确认 /block/<height> 的 TxFeeReward / MinedReward 仍然正常（读路径已回聚合器）。
--   * filscan-syncer：确认日志里 reward-task 正常落库、无列不存在的报错。
--   * 若做了 3b（删列），把 03_validate.sql 从巡检项里移除，否则会一直报「列不存在」。
