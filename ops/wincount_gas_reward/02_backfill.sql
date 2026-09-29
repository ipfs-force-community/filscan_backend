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
-- ===== 实测校验（2026-09-30，只读）=====
--   用 /aggregators/wincount 的 TotalGasReward 与 (epoch, miner) 上的 reward/win_count 逐值比对：
--     epoch 6330000：11 行，11/11 相等
--     epoch 4000000： 5 行， 5/5 相等
--     epoch 6412000： 1 行， 1/1 相等
--   （例：6330000 / f01938674  reward=4188176206443469924 win_count=1
--        ThisEpochReward=20939809989941853428 → trunc(T*1/5)=4187961997988370685
--        → 本式 gas_reward=214208455099239，与聚合器返回的 TotalGasReward 逐位相同）
--
-- ===== 已知偏差（必须靠 03_validate.sql 兜住）=====
--   * penalty > 0 的 epoch（含坏消息的罚没）：本式算出的 gas_reward 偏小 penalty。
--   * 奖励 actor 余额不足以支付 totalReward 的 epoch：totalReward 被截到余额、blockReward 被重算（:103-107）
--     ⇒ 本式失真。这两种情形通常表现为 gas_reward < 0 或明显离群，03_validate.sql 会把负值全部列出。
--   * 一个 epoch 内同一矿工多个块时，blockReward 是「逐块各自截断后求和」，本式是「先求和后截断」，
--     理论上最多差 (块数 - 1) 个 attoFIL（可忽略，但严格比对时别拿它当 0 误差）。
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
