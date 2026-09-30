-- ops/wincount_gas_reward/02_backfill.sql
--
-- 历史行 gas_reward 回填 —— **纯 PG，不碰 mongo / 聚合器**。
--
-- ===== 公式 =====
--
--   gas_reward(epoch, miner) = miner_rewards.reward - trunc(ThisEpochReward(epoch) * win_count / 5)
--
-- 推导（specs-actors v0.9.15 actors/builtin/reward/reward_actor.go:90-140）：
--   AwardBlockReward:
--     blockReward = ThisEpochReward * WinCount / ExpectedLeadersPerEpoch      -- :98-99，整数除法（截断）
--     totalReward = blockReward + GasReward                                  -- :100
--     penalty     = min(params.Penalty, totalReward)                          -- :110
--     rewardPayable = totalReward - penalty                                   -- :113
--     AddLockedFund(miner, rewardPayable)                                     -- :118
--   而聚合器 miners_blockreward（= 本仓 chain.miner_rewards.reward 的落库来源）取的正是
--   `Msg.From:"02" / Msg.Method:14 (ApplyRewards) / Depth:2` 消息的 Value = rewardPayable。
--   ⇒ reward = blockReward + GasReward - penalty ⇒ GasReward = reward - blockReward + penalty。
--   本式忽略 penalty（正常出块 penalty = 0）。
--
--   ExpectedLeadersPerEpoch = 5（builtin 常量，全链固定）。
--   ThisEpochReward 取自奖励 actor（f02）在该 epoch 的状态：chain.builtin_actor_states.state->>'ThisEpochReward'。
--
-- ===== 实测校验（2026-09-30，全部对拍聚合器真值）=====
--   小样本逐值比对（只读，**走生产 trunc 式**）：
--     epoch 6330000：11 行 → 相等 10、差 +1（真值更大）1、其它 0
--     epoch 4000000： 5 行 → 相等 0、差 +1 5、其它 0
--     epoch 6412000： 1 行 → 相等 1、差 +1 0、其它 0
--   （例：6330000 / f01938674  reward=4188176206443469924 win_count=1
--        ThisEpochReward=20939809989941853428
--        → 本式 = 4188176206443469924 − trunc(20939809989941853428*1/5)
--               = 4188176206443469924 − 4187961997988370686 = 214208455099238
--        → 聚合器真值 TotalGasReward                        = 214208455099239
--        ⇒ 本行属「差 +1」那一类，成因见下一节）
--   ⚠ 本节早先版本记的「11/11 / 5/5 相等」是用**精确除法手算**得出的，未走生产 trunc 式；
--     已按生产式重测更正（PG 实测：trunc(20939809989941853428::numeric/5) = …686，
--     而精确值 = …685）。**引用本文件的历史数字时以此处为准。**
--
--   大样本逐行对拍（59 行 / 12 个 epoch，跨高度 3.5M–6.4M）：
--     差 0（完全相等）= 47 行；差 +1 attoFIL（真值比本式大 1）= 12 行；其它差值 = 0 行。
--   ⇒ **本式与真值只有两种关系：完全相等，或比真值小 1 attoFIL。没有第三种。**
--      精度结论：±1 attoFIL（= 1e-18 FIL），可用。
--
-- ===== ⚠ 那 +1 的成因已定位：PG 的 numeric 除法**四舍五入**，不是截断 =====
--   `trunc(a / 5)` 里的 `a / 5` 先被 PG 算成一个 numeric，而 PG 的除法只保留约 16 位有效数字
--   （select_div_scale：rscale = 16 − 商权重，被夹到 ≥ 0）⇒ 本式的 T 有 20 位、商权重 ≈ 19
--   ⇒ rscale = 0 ⇒ **除法结果被四舍五入到整数，`trunc()` 此时已无事可做**。
--   ⇒ 本式的减数 = round(T·wc/5)，链上是 floor(T·wc/5)，两者差 ∈ {0, 1}（小数部分 ≥ .5 时差 1）
--   ⇒ 这**恰好**解释实测的「差 0 / 差 +1，没有第三种」，也解释负值里 98.6% 恰好 = -1。
--
--   可复现实证（本地 PG 18 实测，两行 SQL 即可）：
--     select 20939809989941853428::numeric/5;          -- 4187961997988370686 ← 被进位
--     select trunc(20939809989941853428::numeric/5);   -- 4187961997988370686 ← trunc 截不掉（已是整数）
--     select div(20939809989941853428::numeric, 5);    -- 4187961997988370685 ← **精确**（PG 的截断整数除法）
--     -- 拿下面的生产对拍复核：
--     select 4188176206443469924::numeric - trunc(20939809989941853428::numeric*1/5); -- 214208455099238 ← 本式，比真值小 1
--     select 4188176206443469924::numeric - div(20939809989941853428::numeric*1, 5);  -- 214208455099239 ← 真值（聚合器 TotalGasReward）
--   触发条件：商 ≥ 16 位有效整数位（T ≳ 1e16）。主网奖励 actor 的 ThisEpochReward 有 20 位 ⇒ **恒触发**。
--
--   ⇒ **精确写法**（一行之差，建议下次重跑回填时启用；本次生产用的是 trunc(.../5)，保持原样以便复现现状）：
--        set gas_reward = r.reward - div(ter.this_epoch_reward * w.win_count, 5)
--      div(numeric, numeric) 是 PG 的**截断整数除法**，与链上的整数除法同语义 ⇒ 差恒为 0，
--      「= -1」的行不会再产生（05 的 `-1 → 0` 规则对历史行仍然需要）。
--   ⚠ 换用 div() 后重算的值与现存（已归一化）的行最多差 1 attoFIL；且负值探针会失效（不再有负值）
--     ⇒ 要么整体切换、要么整体不切，别只切一半。
--
-- ===== 负值：实测分布与处理（**不要把负值当成罕见的 penalty 边界**）=====
--   生产回填后出现 2,015,596 行负值。分布是双峰的，不是「罕见的异常」：
--     * 1,987,583 行（98.6%）**恰好 = -1** —— 这是上面 PG 除法四舍五入的边界，**不是模型失真**。
--       由上面的规律可**证明**：真值 ∈ {本式, 本式+1} 且真值 ≥ 0 ⇒ 本式 = -1 时真值必为 0
--       ⇒ 这些行置 0 是**精确的**（不是近似）。
--     * 28,013 行（1.4%）≤ -2 —— 这些才是**真偏差**，对应下面「已知偏差的成因」，
--       量级 ≤ ~1e-6 FIL；不可信 ⇒ 置 NULL，让读路径按设计**整请求回落聚合器**
--       （agg_pg_reward.go 的 incompleteGasRewardRow），代价只是慢、不产生错值。
--   ⇒ 归一化已固化成 `05_normalize_negatives.sql`（前置计数 + 两条 UPDATE + 收口记账），
--     断言与三组计数见 `06_validate_after_normalize.sql`（negative_rows 必须 0）。
--   ⇒ 因此 `03_validate.sql` 的 negative_rows **非 0 不等于模型被打破**：
--     先按「= -1 / ≤ -2」拆开看，98.6% 是取整边界（跑 05 即可），
--     只有 ≤ -2 的那批才需要人工核对聚合器返回值。
--
-- ===== 已知偏差的成因（对应上面「真偏差」那 1.4%；不是负值的主要来源）=====
--   * penalty > 0 的 epoch（含坏消息的罚没）：本式算出的 gas_reward 偏小 penalty。
--   * 奖励 actor 余额不足以支付 totalReward 的 epoch：totalReward 被截到余额、blockReward 被重算（:103-107）
--     ⇒ 本式失真。上面两类才是「明显离群 / ≤ -2」的来源。
--   * 一个 epoch 内同一矿工多个块时，blockReward 是「逐块各自截断后求和」，本式是「先求和后截断」
--     ⇒ 本式 ≤ 真值，差 ∈ {0, 1, …, 块数−1} attoFIL。这是与上面 PG 除法**同方向**的次要项；
--     实测差值从未超过 1 ⇒ 两个机制合起来也只产生 ±1。
--
-- ===== 为什么必须按分区跑、且只能 UPDATE =====
--   * 本表带 INSERT 规则（range_insert_action_rule），**不能写 ON CONFLICT**：
--     PG 报 `ON CONFLICT clause is not supported on tables with INSERT rules`。
--     回填只改已存在的行 ⇒ UPDATE；写路径保持纯 INSERT（reward_task.go）。
--   * 按周分区跑（每分区 ~2 万 epoch / 5 万行）：单条 UPDATE 只碰一个分区，PG 能裁剪，
--     单次事务小、可中断、可续跑。`gas_reward is null` 让脚本幂等 —— 重跑只补没补过的行。
--   * ⚠ 本表可能对同一 (epoch, miner) 有**重复行**（索引非唯一且 `on only`，写路径是纯 INSERT）。
--     这里不去重：重复行的 win_count 相同 ⇒ 算出的 gas_reward 也相同 ⇒ 一起更新是正确的；
--     读路径（SQLMinerWinCountsRange）才做 DISTINCT ON 去重。
--
-- 占位符（由 backfill.sh 按分区边界代入）：
--   :lo  = 分区下界（含）
--   :hi  = 分区上界（不含）
-- 直接手跑时把 :lo / :hi 换成数字即可。
--
-- 回填跑完的**必做后续**（顺序固定）：
--   1) 03_validate.sql —— 看 NULL 行数（决定读路径是否回落）与 negative_rows；
--   2) negative_rows 非 0 ⇒ 05_normalize_negatives.sql（-1 精确置 0，≤ -2 置 NULL）；
--   3) 06_validate_after_normalize.sql —— 断言 negative_rows = 0 并对账。

with ter as (select epoch, (state ->> 'ThisEpochReward')::numeric as this_epoch_reward
             from chain.builtin_actor_states
             where actor = 'f02'
               and epoch >= :lo
               and epoch < :hi
               and state ->> 'ThisEpochReward' is not null),
     -- 与读路径同口径去重：同 (epoch, miner) 多条时取 block_time 最新的一条
     r as (select distinct on (epoch, miner) epoch, miner, reward
           from chain.miner_rewards
           where epoch >= :lo
             and epoch < :hi
           order by epoch, miner, block_time desc nulls last),
     u as (
         update chain.miner_win_counts w
             set gas_reward = r.reward - trunc(ter.this_epoch_reward * w.win_count / 5)
             from r
                      join ter on ter.epoch = r.epoch
             where w.epoch = r.epoch
               and w.miner = r.miner
               and w.gas_reward is null
               and w.win_count is not null
               and w.epoch >= :lo
               and w.epoch < :hi
             returning 1)
select count(*) as rows_updated
from u;

-- ===== 不持久风险（回填不是「一劳永逸」）=====
--   1) 分区被 drop 后由规则 chain_partition_create 重建 ⇒ 该周数据（含已回填值）全丢，回填不会自动重来。
--      判据：03_validate.sql 的 NULL 行数重新变成非 0。
--   2) 用旧备份/旧脚本 `insert ... select` 恢复分区会带回 NULL 行。
--   3) 同步器 RollBack 到某高度会 DeleteWinCounts 后重插 —— 只要同步器已经是**新版本**，
--      重插的行自带 gas_reward；但若回滚发生在旧版本二进制上，那批行会重新变成 NULL。
--   ⇒ 结论：把 03_validate.sql 的「NULL 行数」当成常规巡检项；非 0 就重跑本脚本（幂等）。
