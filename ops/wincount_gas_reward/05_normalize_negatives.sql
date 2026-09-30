-- ops/wincount_gas_reward/05_normalize_negatives.sql
--
-- 回填后的**负值归一化**（2026-09-30 生产实测后固化；当日在生产已执行，本文件是等价复现）。
--
-- ===== 为什么需要这一步 =====
--
-- 02_backfill.sql 的公式与聚合器真值的关系已被实测钉死（见 README §「上线实录（2026-09-30）」）：
--
--     真值 ∈ {公式, 公式 + 1}        —— 差 0 或 +1 attoFIL，**没有第三种情况**
--     （59 行 / 12 个 epoch 逐行对拍：差 0 = 47 行，差 +1 = 12 行，其它 = 0 行）
--
-- 由此得到两条结论，一条是**可证的精确**，一条是**真偏差**：
--
--   (1) 公式 = -1 的行 ⇒ 真值 ∈ {-1, 0}；再取口径前提「真值 ≥ 0」
--       ⇒ 真值必为 0 ⇒ 把公式 = -1 的行置 0 是**精确的**，不是近似。
--       实测：2,015,596 个负值行里 1,987,583 行恰好 = -1（98.6%）。
--   (2) 其余负值（公式 ≤ -2）用上面的规律解释不了 ⇒ 是**真偏差**
--       （怀疑 penalty > 0 或奖励 actor 余额不足；量级 ≤ ~1e-6 FIL）。
--       这批值不可信 ⇒ 置 NULL，让读路径按设计**整请求回落聚合器**。
--       实测：28,013 行。
--
-- ⚠ 前提显式化：「真值 ≥ 0」是结论 (1) 的前提。它来自聚合器侧口径
--   （TotalGasReward = $sum 链上 AwardBlockReward 的 GasReward 参数）。
--   若将来出现真值 < 0 的证据，则 -1 行置 0 需要重新核对，本步骤也要重做。
--
-- ===== 为什么「其余负值」置 NULL 而不是置 0 =====
--
--   0 是聚合器的**合法取值**（线上实测 epoch 6330000 就有 10 个 (epoch,miner) 的
--   TotalGasReward = 0），拿 0 顶会**静默改口径**：acl_block_chain.GetBlockDetails 用它算
--   TxFeeReward = TotalGasReward、MinedReward = TotalBlockReward − TotalGasReward ⇒ 金额算错。
--   NULL 则是读路径的显式「不可用」信号（agg_pg_reward.go 的 incompleteGasRewardRow）
--   ⇒ 整个请求回落聚合器，代价只是慢，不产生错值。
--
-- ===== 可逆性（重要）=====
--   * 置 NULL **可逆**：重跑 02_backfill.sql 会按同一公式把这些 NULL 行重新算出来
--     （它的 WHERE 就是 `gas_reward is null`），再重跑本文件即可再归一化。
--   * 置 0 **不可逆**：执行后「原来是 -1」与「真值本来就是 0」不再可区分。
--     要复核只能重跑 02_backfill.sql 的公式（先把这些行置 NULL 再算）。
--
-- ===== 执行 =====
--   /root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/05_normalize_negatives.sql
--   然后跑 06_validate_after_normalize.sql 断言 negative_rows = 0。
--
--   本文件自带 `\set ON_ERROR_STOP on`（见下）⇒ 任一步失败立刻停，不会带着半截状态往下跑。
--   （必须用 `-f` 执行；`-c` 会把 `\set` 当普通文本，backfill.sh 那种 `-c` 调用方式不适用本文件。）
--
--   幂等：两条 UPDATE 都带 `gas_reward < 0` 谓词 ⇒ 第二次跑更新 0 行。
--   可中断：整表两条 UPDATE 各是一个事务（生产实测负值总量 200 万行级，分钟级内完成）。
--   ⚠ 单事务会把 200 万行一次性写进 WAL / 脏页。若要更保守，用文件末尾的
--     **逐分区变体**（:lo / :hi，与 backfill.sh / 02_backfill.sql 同约定）。
--
--   ⚠ **两条语句的顺序不可颠倒**：步骤 2 的 `gas_reward < 0` 会吃掉步骤 1 要处理的
--     那批 `-1`（把本该精确置 0 的行也置成 NULL）。先 1 后 2。

\set ON_ERROR_STOP on

-- ============================================================
-- 步骤 0：前置计数（只读；先看清影响面，再决定要不要写）
-- ============================================================
-- 判据（对照 2026-09-30 生产实测）：
--   negative_rows       = 2,015,596
--   exactly_minus_one   = 1,987,583   （98.6% ⇒ 归一化后是**精确**的）
--   true_deviation_rows =    28,013   （1.4%  ⇒ 置 NULL 回落聚合器）
--   null_rows           = 2,330,460   （其中 28,013 会由本步骤产生，见 06 的记账）
-- 若 negative_rows = 0 ⇒ 本步骤无需执行（已归一化过或回填没产生负值）。
select count(*) filter (where gas_reward < 0)  as negative_rows,
       count(*) filter (where gas_reward = -1) as exactly_minus_one,
       count(*) filter (where gas_reward < -1) as true_deviation_rows,
       count(*) filter (where gas_reward is null) as null_rows
from chain.miner_win_counts;

-- ============================================================
-- 步骤 1：公式 = -1 ⇒ 真值必为 0（**精确**，置 0）
-- ============================================================
-- 生产实测：1,987,583 行。
update chain.miner_win_counts set gas_reward = 0 where gas_reward = -1;

-- ============================================================
-- 步骤 2：其余负值 = 真偏差（不可信，置 NULL ⇒ 读路径整请求回落聚合器）
-- ============================================================
-- 生产实测：28,013 行。必须在步骤 1 之后跑。
update chain.miner_win_counts set gas_reward = null where gas_reward < 0;

-- ============================================================
-- 步骤 3：收口
-- ============================================================
-- 断言 + 三组计数（negative_rows 必须 0）：
--   /root/ltr_pg.sh -At -F'|' -v ON_ERROR_STOP=1 -f ops/wincount_gas_reward/06_validate_after_normalize.sql
-- 记账（2026-09-30 生产实测，可用来对账）：
--   回填写入 14,145,916 行 − 置 NULL 28,013 行 = 非 NULL 14,117,903 行  ✓
--   负值   2,015,596 行 = 置 0 1,987,583 行 + 置 NULL 28,013 行        ✓
--   NULL 总计 2,330,460 行 = 28,013（真偏差）+ 2,302,447（未回填/未覆盖高度）
--   gas_reward = 0 共 4,971,087 行（含本步骤置 0 的 1,987,583 行；均与真值差 ≤ 1 attoFIL）

-- ============================================================
-- 逐分区变体（可选；把整表两条 UPDATE 换成 317 次单分区 UPDATE）
-- ============================================================
-- 与 backfill.sh 同约定：调用方把 :lo / :hi 换成分区边界（含 lo、不含 hi），
-- 分区名与边界的取法见 backfill.sh 的 list_partitions_sql。
-- 每条都带 epoch 谓词 ⇒ PG 分区裁剪，只碰一个分区、单事务小、可中断、可续跑。
--
--   update chain.miner_win_counts set gas_reward = 0
--    where gas_reward = -1 and epoch >= :lo and epoch < :hi;
--
--   update chain.miner_win_counts set gas_reward = null
--    where gas_reward < 0  and epoch >= :lo and epoch < :hi;
--
-- 说明：负值只占全表 12%（2.0M / 16.4M 行），整表跑与逐分区跑的总写入量相同，
-- 差别只在「一个事务」还是「317 个事务」。生产当日用的是整表两条语句，已通过。
