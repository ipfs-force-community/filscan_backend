# wincount `TotalGasReward` 落库 / 回填 / 验收 runbook

> 分支：`feat/wincount-gas-reward`　迁移：`migration/36.miner_win_counts_gas_reward.sql`
> 目标：让聚合器端点 `/aggregators/wincount` 的 `TotalGasReward` 在 PG 侧有真值来源，
> 从而可以安全打开开关 `[feature] miner_wincount_read_from_pg`。

---

## 0. 这个值到底从哪来（一句话）

`TotalGasReward` = **`RewardActor.AwardBlockReward` 隐式消息 params 里的 `GasReward`**，按 (epoch, miner) 求和。

* 聚合器管线：`londobell-aggregators/pool-monitor/wincount_zl.js`
  `$match {Epoch∈[s,e), Depth:1, Msg.From:"00", Msg.To:"02", Msg.Method:2}` → `$lookup Message` →
  `$group {_id: Message.Detail.Params.Miner, TotalWinCount: $sum …WinCount, TotalGasReward: $sum {$toDecimal: …GasReward}}`
* params 类型：`specs-actors v0.9.15 actors/builtin/reward/reward_actor.go:56`
  `AwardBlockRewardParams{Miner, Penalty, GasReward, WinCount}` —— **WinCount 与 GasReward 是同一条消息的两个字段**。
* 同步器 `modules/syncer/chain/reward_task/reward_task.go` 本来就调同一个端点（`ctx.Agg().WinCount(epoch, epoch+1)`），
  `TotalGasReward` 一直在响应里，只是 `po.MinerWinCount` 没有列可以落 ⇒ PG 路径该字段恒为 0。
* 它唯一的消费点：`acl_block_chain.GetBlockDetails` 用它算
  `TxFeeReward = TotalGasReward`、`MinedReward = TotalBlockReward − TotalGasReward`
  （`assembler_block_chain_info.go:42-58`）⇒ 用 0 顶会把区块详情页的两个金额算错。

**结论：这个值只能在同步器侧从聚合器（mongo）取，PG 里没有等价来源 —— 但不需要新增任何数据源，
因为同步器已经在同一次请求里拿到它了。** 历史行则由 PG 侧公式回填（见 §3）。

---

## 1. 交付物

| 文件 | 作用 |
|---|---|
| `migration/36.miner_win_counts_gas_reward.sql` | `alter table chain.miner_win_counts add column gas_reward numeric`（幂等）+ 3 条后检 |
| `ops/wincount_gas_reward/01_preflight.sql` | 上线前只读体检（分区/规则/覆盖/数据源在位） |
| `ops/wincount_gas_reward/02_backfill.sql` | 历史回填 UPDATE（单分区、幂等、纯 PG） |
| `ops/wincount_gas_reward/backfill.sh` | 按周分区跑回填（默认拒绝执行，必须显式 `--dry-run`/`--yes`） |
| `ops/wincount_gas_reward/03_validate.sql` | 回填进度 / 负值 / 抽样核对 |
| `ops/wincount_gas_reward/90_rollback.sql` | 回滚（关开关 → 回滚二进制 → 再动 DDL） |
| 代码 | `po.MinerWinCount.GasReward`、`reward_task.toMinerWinCount`、`SQLMinerWinCountsRange`、`bo.AccWinCount.GasReward/TotalRows/GasRewardRows`、`agg_pg_reward.WinCount`（含未回填回落）、parity 工具逐值比对 |

**代码侧的关键设计**：`agg_pg_reward.WinCount` 只要发现区间里还有 `gas_reward IS NULL` 的行
（`incompleteGasRewardRow`）就**整请求回落聚合器**并打 Warn。于是：

* 未回填区间 = **今天的行为**（走聚合器，慢但不改口径）；
* 已回填 / 新写入区间 = 走 PG；
* 任何时刻都不存在「拿 0 顶过去」的窗口 ⇒ 开开关不会产生口径回归，回填进度只影响快慢。

---

## 2. 上线顺序（不可颠倒）

```bash
# ① 只读体检
/root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/01_preflight.sql

# ② DDL（加列，级联到全部分区）
/root/ltr_pg.sh -At -f migration/36.miner_win_counts_gas_reward.sql
#   跑完把文件里的「后检 1 / 后检 2」各跑一次：必须 0 / 空

# ③ 回填（先 dry-run 看影响面）
export PSQL_CMD=/root/ltr_pg.sh
ops/wincount_gas_reward/backfill.sh --dry-run
ops/wincount_gas_reward/backfill.sh --since <起始高度> --sleep 2 --yes
#   317 个周分区；建议分批（--max-partitions N --sleep S），每分区只碰一个分区、单事务

# ④ 校验
/root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/03_validate.sql
#   看两点：negative_rows 必须 0；NULL 行数 = 未覆盖范围（决定读路径会不会回落）

# ⑤ 先发 filscan-syncer（新版本，写 gas_reward），确认新高度落库带值
/root/ltr_pg.sh -At -F'|' -c "select epoch,miner,win_count,gas_reward from chain.miner_win_counts where epoch = <最新高度> order by miner"
#   gas_reward 应为 NULL 之外的具体数字（0 也是合法值）

# ⑥ 跑 parity 通关（见 §4）
make build-agg-parity && ./bin/agg-parity -c /root/config.toml -endpoints wincount -start <s> -end <e>

# ⑦ 再改 filscan-api 配置打开开关并重启
#   [feature]
#   miner_wincount_read_from_pg = true
```

> ⚠ **不要用 `ON CONFLICT`**：`chain.miner_win_counts` / `chain.miner_rewards` 都带 INSERT 规则
> （`range_insert_action_rule` → `chain.action_*_range_insert(new.*)`），PG 直接拒绝
> `ON CONFLICT clause is not supported on tables with INSERT rules`。写路径保持纯 INSERT，回填只能 UPDATE。
>
> ⚠ **加列必须不带 `ONLY`**：分区父表 `ADD COLUMN` 会级联到全部 317 个分区；写成 `only` 会让分区少一列，
> 规则里的 `INSERT INTO <part> SELECT $1.*` 直接报错 ⇒ 写入路径全挂。

---

## 3. 历史回填：公式、代价、取舍

### 3.1 公式（纯 PG，不碰 mongo / 聚合器）

```
gas_reward(epoch, miner) = miner_rewards.reward − trunc(ThisEpochReward(epoch) × win_count / 5)
```

* `miner_rewards.reward` = 聚合器 `miners_blockreward` 落库值 = `Msg.From:"02"/Method:14(ApplyRewards)` 消息的 Value
  = `rewardPayable = blockReward + GasReward − penalty`（reward_actor.go:98-118）
* `blockReward = trunc(ThisEpochReward × WinCount / 5)`（:98-99，`ExpectedLeadersPerEpoch = 5`）
* `ThisEpochReward` 取自 `chain.builtin_actor_states` 的 `actor='f02'` 行的 `state->>'ThisEpochReward'`

### 3.2 实测校验（2026-09-30 只读，与聚合器逐值比对）

| epoch | 行数 | 相等 |
|---|---|---|
| 6,330,000 | 11 | 11/11 |
| 4,000,000 | 5 | 5/5 |
| 6,412,000 | 1 | 1/1 |

例（6330000 / f01938674）：`reward=4188176206443469924`、`win_count=1`、`ThisEpochReward=20939809989941853428`
→ `trunc(T/5)=4187961997988370685` → 本式 `214208455099239` = 聚合器 `TotalGasReward`（逐位相同）。

### 3.3 已知偏差（`03_validate.sql` 的 `negative_rows` 就是它的探针）

* `penalty > 0`（含坏消息的罚没）⇒ 本式**偏小 penalty**；
* 奖励 actor 余额不足以支付 `totalReward` ⇒ `totalReward` 被截到余额、`blockReward` 被重算（:103-107）⇒ 失真；
* 同一 epoch 同一矿工多块时，`blockReward` 是「逐块截断后求和」vs 本式「先求和后截断」，
  理论差 ≤ (块数−1) attoFIL。

前两种通常表现为 `gas_reward < 0`，所以校验脚本把负值全部列出。**发现负值不要直接用**：
那批高度的 `gas_reward` 需要拿聚合器返回值核对（`filscan-agg-parity -endpoints wincount -start X -end Y`）。

### 3.4 代价与取舍

* **不做回填也完全可以上线**：读路径对未回填区间自动回落聚合器 ⇒ 老高度行为与今天完全一致，
  只有新高度走 PG。代价是「历史高度仍然慢」。
* 回填是 **317 次单分区 UPDATE**，每分区约 5 万行、秒级；不碰 mongo、不碰聚合器主机。
  建议分批 + `--sleep`，避开 PG 自身的同步窗口。
* **不建议**改从 mongo 逐 epoch 回填：那需要 640 万次聚合器调用或新增一条按 (epoch,miner) 分组的管线
  （要重建 londobell-api 并部署到聚合器主机），代价远高于上面的纯 PG 公式。

### 3.5 「回填不持久」风险（必须当巡检项）

回填写的是**行数据**，不是约束，以下情况会让它消失：

1. 分区被 drop 后由规则 `chain_partition_create` 重建 ⇒ 该周数据全丢，回填不会自动重来；
2. 用旧备份 / 旧脚本 `insert … select` 恢复分区会带回 NULL 行；
3. 同步器 RollBack 到某高度会 `DeleteWinCounts` 再重插 —— 新版本二进制重插的行自带 `gas_reward`；
   若回滚发生在旧版本二进制上，那批行会重新变成 NULL。

⇒ 把 `03_validate.sql` 的「NULL 行数」当常规巡检项；非 0 就重跑 `backfill.sh`（幂等，只补 NULL）。

---

## 4. 验收步骤

1. **单测**（本分支已全绿）
   ```bash
   go test ./modules/common/infra/dal/... ./modules/filscan/biz/browser/... \
            ./modules/filscan/service/rewardparity/... ./modules/syncer/chain/reward_task/...
   ```
2. **集成测试**（一次性 PG，验证「真 PG + 真聚合器客户端」两条路径逐字段相等，含未回填回落）
   ```bash
   FILSCAN_PG_PARITY_DSN='host=127.0.0.1 port=5433 user=postgres dbname=filscan_probe sslmode=disable' \
     go test ./modules/filscan/biz/browser/ -run TestPgReward -v
   ```
3. **口径通关（线上只读）** —— 必须 `TotalGasReward` 差异 0、Unresolved 为空：
   ```bash
   ./bin/agg-parity -c /root/config.toml -endpoints wincount -start <s> -end <e>
   # 期望：字段差异 0 条；点位缺失 0/0；且**不再**出现
   #      「TotalGasReward：本区间仍有 gas_reward IS NULL 的行」这条 Unresolved
   ```
   若仍出现该 Unresolved ⇒ 该区间还没回填（读路径会回落聚合器，功能正确、只是慢），先回填再复跑。
4. **端到端**（开关打开后）
   * `/block/<height>` 的 `TxFeeReward` / `MinedReward` 与开关打开前逐值一致
     （取一个已回填高度 + 一个未回填高度各测一次：前者走 PG、后者回落聚合器，两者都应一致）；
   * `filscan-api` 日志里未回填区间应出现
     `read wincount from pg is incomplete (...) ... fallback to aggregator` 的 Warn —— 这是**预期行为**。
5. **回滚演练**：按 `90_rollback.sql` 的逆序走一遍（关开关 → 回滚二进制 → 再考虑动 DDL）。

---

## 5. 本地验证记录（本次交付已实测）

在一次性本地 PG 18 集群上复刻了线上的「分区父表 + `range_insert_action_rule` + `SELECT $1.*`」形态后：

* `migration/36` 执行后：分区缺列数 = 0、`gas_reward` 列序号在父表与各分区一致、通过规则 INSERT 4 列正常落位；
* `backfill.sh --dry-run / --yes` 正常，二次执行 0 行（幂等）；重复行（同一 (epoch,miner) 两份）被一致更新；
* 缺 `ThisEpochReward` 的高度留在 NULL（→ 读路径回落），故意构造的失真行被 `negative_rows` 捕获；
* `SQLMinerWinCountsRange` 在完全回填区间给出 `total_rows == gas_reward_rows`，含 NULL 行时 `7/6` ⇒ 触发回落；
* `03_validate.sql` 的 epoch→日期分桶与线上分区命名一致（epoch 6,326,640 → `2026_08_31`）。
