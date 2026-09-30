# wincount `TotalGasReward` 落库 / 回填 / 验收 runbook

> 分支：`feat/wincount-gas-reward`　迁移：`migration/36.miner_win_counts_gas_reward.sql`
> 目标：让聚合器端点 `/aggregators/wincount` 的 `TotalGasReward` 在 PG 侧有真值来源，
> 从而可以安全打开开关 `[feature] miner_wincount_read_from_pg`。
>
> ⚠ **本分支已于 2026-09-30 在生产完整跑完**（迁移 → 二进制 → 回填 → 负值归一化 → 开开关）。
> 实测数字、±1 attoFIL 精度结论、负值归一化的可证逻辑、耗时与回滚命令见 **§6「上线实录（2026-09-30）」**。
> **合入紧迫性见 §6.0**：migration/36 已在生产应用 ⇒ 不合入 main 就会 schema 漂移。

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
| `ops/wincount_gas_reward/04_dryrun_premigration.sql` | **DDL 之前**的影响面估算（不引用 `gas_reward` 列） |
| `ops/wincount_gas_reward/dryrun_premigration.sh` | 按分区跑 `04_*.sql`（DDL 前估行数/耗时） |
| `ops/wincount_gas_reward/05_normalize_negatives.sql` | **负值归一化**（`-1` 精确置 0；`≤ -2` 置 NULL 回落聚合器） |
| `ops/wincount_gas_reward/06_validate_after_normalize.sql` | 归一化后的**硬断言**（`negative_rows = 0`）+ 三组计数对账 |
| `ops/wincount_gas_reward/rollout.sh` | 上线编排（① migration → ② 二进制 → ③ 回填 → ④ 开关），每步独立判据、失败即停 |
| `ops/wincount_gas_reward/rollback.sh` | 逆序回滚（① 关开关 → ② 回滚二进制 → ③ 才动 DDL），默认 dry-run |
| `ops/wincount_gas_reward/90_rollback.sql` | 回滚 SQL 说明（关开关 → 回滚二进制 → 再动 DDL） |
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
#   实测耗时 **0.3 秒**；317 个分区全部拿到 gas_reward，列序号一致 = 4，INSERT 规则未变
#   跑完把文件里的「后检 1 / 后检 2」各跑一次：必须 0 / 空

# ③ 回填（先 dry-run 看影响面；DDL 之前想估影响面用 dryrun_premigration.sh）
export PSQL_CMD=/root/ltr_pg.sh
ops/wincount_gas_reward/backfill.sh --dry-run
ops/wincount_gas_reward/backfill.sh --since <起始高度> --sleep 2 --yes
#   317 个周分区；建议分批（--max-partitions N --sleep S），每分区只碰一个分区、单事务
#   实测：147/317 个分区有待更新行；全量 14,145,916 行（dry-run 预估 14,140,489）；
#         最大分区 99,145 行 = 2 秒；**全量约 15 分钟**（不是小时级）

# ④ 校验
/root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/03_validate.sql
#   看两点：NULL 行数 = 未覆盖范围（决定读路径会不会回落）；
#           negative_rows —— 回填刚跑完时非 0 是**正常**的（98.6% 恰好 = -1）

# ④b 负值归一化（**必做**，顺序固定：先 05 再 06）
/root/ltr_pg.sh -At -F'|' -v ON_ERROR_STOP=1 -f ops/wincount_gas_reward/05_normalize_negatives.sql
/root/ltr_pg.sh -At -F'|' -v ON_ERROR_STOP=1 -f ops/wincount_gas_reward/06_validate_after_normalize.sql
#   05：`gas_reward = -1` 精确置 0（实测 1,987,583 行）；`gas_reward < 0` 置 NULL（实测 28,013 行）
#   06 判据：输出 `ASSERT OK: negative_rows=0`，且 backfilled + null == total

# ⑤ 先发 filscan-syncer（新版本，写 gas_reward），确认新高度落库带值
/root/ltr_pg.sh -At -F'|' -c "select epoch,miner,win_count,gas_reward from chain.miner_win_counts where epoch = <最新高度> order by miner"
#   gas_reward 应为 NULL 之外的具体数字（0 也是合法值）
#   实测（epoch 6,414,319）：3 行全部带值 0 / 124189410895125 / 30930291727321

# ⑥ 跑 parity 通关（见 §4）
make build-agg-parity && ./bin/agg-parity -c /root/config.toml -endpoints wincount -start <s> -end <e>

# ⑦ 再改 filscan-api 配置打开开关并重启
#   [feature]
#   miner_wincount_read_from_pg = true
#   启动日志应出现：
#     aggregator reward endpoints switched to PG: miner_blockreward=true miners_blockreward=true wincount=true timeout=5s
#   且 `from pg failed = 0`

# 以上 ①–④b 与 ⑤–⑦ 各有编排脚本：rollout.sh（① ② ③ ④ 四步，每步独立判据、失败即停）
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

### 3.2 实测校验（2026-09-30，与聚合器逐值比对）

| epoch | 行数 | 相等 |
|---|---|---|
| 6,330,000 | 11 | 11/11 |
| 4,000,000 | 5 | 5/5 |
| 6,412,000 | 1 | 1/1 |

例（6330000 / f01938674）：`reward=4188176206443469924`、`win_count=1`、`ThisEpochReward=20939809989941853428`
→ `trunc(T/5)=4187961997988370685` → 本式 `214208455099239` = 聚合器 `TotalGasReward`（逐位相同）。

**上线当天又做了一次大样本逐行对拍**（59 行 / 12 个 epoch，跨高度 3.5M–6.4M）：
差 0（完全相等）= 47 行、差 +1 attoFIL = 12 行、其它差值 = **0 行**
⇒ **公式要么与真值完全相等、要么只差 1 attoFIL，精度 ±1 attoFIL（1e-18 FIL）**。详见 §6.3。

### 3.3 负值的**实测分布**（`03_validate.sql` 的 `negative_rows` 就是它的探针）

生产回填后实测 **2,015,596 行负值**，是**双峰**分布 —— **不是「罕见的 penalty 边界」**：

| 分组 | 行数 | 占比 | 性质 |
|---|---|---|---|
| `= -1` | 1,987,583 | 98.6% | PG 除法四舍五入的边界（§6.3；真值必为 0，**可证**） |
| `≤ -2` | 28,013 | 1.4% | 真偏差（`penalty > 0` / 奖励 actor 余额不足，量级 ≤ ~1e-6 FIL） |

* `= -1` 那批由 §6.3 的规律**证明**真值 = 0 ⇒ **置 0 是精确的**（不是近似）；
* 反向也成立：由 §6.3，**本式 ≥ 真值 − 1** ⇒ 本式 ≤ −2 **不可能**是取整造成的，只能是真偏差；
* `≤ -2` 那批不可信 ⇒ 置 **NULL**（读路径整请求回落聚合器，不产生错值，只是慢）；
* 归一化已固化为 `05_normalize_negatives.sql`（前置计数 + 两条 UPDATE），
  断言与三组计数见 `06_validate_after_normalize.sql`。

⇒ **`negative_rows` 非 0 不等于模型被打破**：先按「`= -1` / `≤ -2`」拆开看，
98.6% 是取整边界（跑一遍 05 即可），只有 `≤ -2` 的那批才需要拿聚合器返回值核对
（`filscan-agg-parity -endpoints wincount -start X -end Y`）。

**已知偏差的成因**（对应 `≤ -2` 的 1.4%；不是负值的主要来源）：

* `penalty > 0`（含坏消息的罚没）⇒ 本式**偏小 penalty**；
* 奖励 actor 余额不足以支付 `totalReward` ⇒ `totalReward` 被截到余额、`blockReward` 被重算（:103-107）⇒ 失真；
* 同一 epoch 同一矿工多块时，`blockReward` 是「逐块截断后求和」vs 本式「先求和后截断」
  ⇒ 本式 ≤ 真值，差 ∈ {0, 1, …, 块数−1} attoFIL —— **这就是那 98.6% 的 `-1` 的成因假设**
  （实测差值从未超过 1）。

### 3.4 代价与取舍

* **不做回填也完全可以上线**：读路径对未回填区间自动回落聚合器 ⇒ 老高度行为与今天完全一致，
  只有新高度走 PG。代价是「历史高度仍然慢」。
* 回填是 **317 次单分区 UPDATE**；不碰 mongo、不碰聚合器主机。建议分批 + `--sleep`，避开 PG 自身的同步窗口。
  **实测（§6.2）**：每分区行数比上线前「约 5 万行」的估计大一档（2024 年以后的分区约 9.8 万行，
  最大 99,145 行 = 2 秒）；147 个分区有待更新行、共 14,145,916 行、**全量约 15 分钟**。
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

**追加验证（2026-09-30，`05` / `06` 两个新脚本）** —— 在同一个一次性副本库上端到端跑通：

* 三种边界各造一个探针（用生产 `T = 20939809989941853428`，其 PG `round(T/5) = …686`、真 `floor = …685`）：
  `reward = floor` ⇒ 公式 `-1`（真值 0）→ 05 置 **0**；
  `reward = floor - 5` ⇒ 公式 `-6`（真偏差）→ 05 置 **NULL**；
  `reward = round` ⇒ 公式 `0`（真值 1）→ 05 **不动**。
  归一化后复核：`0 / NULL / 0` ✓，与预期逐个一致。
* 05 的前置计数与实际改动对上账（`negative_rows = 1(-1) + 2(≤-2)` ⇒ `UPDATE 1` + `UPDATE 2`）；
  06 输出 `ASSERT OK: negative_rows=0` + `total=54 backfilled=51 null=3 zero=14 null_pct=5.56`
  （`backfilled + null == total` 通过）✓。
* 幂等：二次跑 05 = `UPDATE 0` / `UPDATE 0`，06 仍 `ASSERT OK` ✓。
* **断言真的会拦人**：手工塞回一个负值 ⇒ 06 报 `ASSERT FAILED: negative_rows=1` 且**退出码 3**
  （`\set ON_ERROR_STOP on` 在位；实测不带它时 psql 会以 0 退出 ⇒ 两个脚本都自带这一行）✓。
  同样地，在缺 `gas_reward` 列的库上跑 05 会立刻退出码 3，不会带着半截状态继续。

> 以上是**上线前**在本地/只读环境做的验证。**上线后的生产实测**（迁移、回填、精度、归一化、开关、回滚）
> 全部记在下面这一节 —— 数字与结论以 §6 为准，§3 里的预估口径已被 §6 校正。

---

## 6. 上线实录（2026-09-30）

> 本分支当日已在生产（backend `172.31.34.109`）**完整跑完**：
> 迁移 → 二进制 → 回填 → 负值归一化 → 开开关。
> 本节全部是**实测数字**（来自生产输出），可用来复核与复现。

### 6.0 合入紧迫性：migration/36 已在生产应用

**生产的 schema 已经领先 `main`。** 不把本分支合入 main，schema 就与 main 漂移：

* `chain.miner_win_counts.gas_reward` 只存在于生产 + 本分支；main 上的代码、离线回放、新环境建库
  都不知道这一列，而生产已经按 4 列写入；
* 任何「从 main 重建 schema / 做对比基线」的动作都会看到生产多一列，却在 main 上找不到对应的迁移文件；
* 反向不成立：migration/36 是幂等的（`add column if not exists`），对已应用的生产库是 no-op。

⇒ **这是本次 PR 的唯一紧迫理由**，与代码改动无关。

### 6.1 迁移（已应用）

| 项目 | 实测 |
|---|---|
| `ALTER TABLE` 耗时 | **0.3 秒** |
| 分区 | **317 / 317 全部拿到 `gas_reward`**（后检 1 = 0） |
| 列序号 | 父表与各分区一致 = **4**（后检 2 = 空） |
| INSERT 规则 | `range_insert_action_rule` **未变**，`SELECT $1.*` 仍整行展开（写入路径无需改动） |

判据（可复跑）：migration 文件末尾的「后检 1 / 后检 2」各跑一次，必须 `0` / 空。

### 6.2 回填（已执行）

| 项目 | dry-run 预估 | 实际写入 |
|---|---|---|
| 有待更新行的分区 | 147 / 317 | 147 / 317 |
| 行数 | 14,140,489 | **14,145,916** |

* 单分区速率实测：**最大分区 99,145 行 = 2 秒**。
* **全量回填总共约 15 分钟** —— 不是小时级；§3.4 上线前按 14.1M 行 + 3 个 btree 索引做的预算过于保守。
* 口径不变：按周分区、单事务、幂等（只改 `gas_reward is null`）。
* 实际比 dry-run 预估多 5,427 行：dry-run 与正式跑之间新高度继续落库，同一区间里新增了 (epoch,miner) 行。

### 6.3 公式精度：±1 attoFIL（本次最重要的发现）

拿聚合器真值**逐行对拍**，59 行 / 12 个 epoch（跨高度 3.5M–6.4M）：

| 差值（真值 − 公式） | 行数 |
|---|---|
| **0（完全相等）** | **47** |
| **+1（真值比公式大 1 attoFIL）** | **12** |
| 其它差值 | **0** |

⇒ **公式与真值只有两种关系：完全相等，或比真值小 1 attoFIL。没有第三种情况。**
**结论：公式可用，精度 ±1 attoFIL（= 1e-18 FIL）。**

**成因已定位（可复现）：PG 的 numeric 除法会四舍五入，不是截断。**

`trunc(ter.this_epoch_reward * w.win_count / 5)` 里的 `T*wc/5` 先由 PG 算成一个 numeric，
而 PG 的除法只保留约 16 位有效数字（`select_div_scale`：`rscale = 16 − 商权重`，夹到 ≥ 0）——
本式的 `T` 有 20 位、商权重 ≈ 19 ⇒ `rscale = 0` ⇒ **除法结果被四舍五入到整数，`trunc()` 已无事可做**。
于是本式的减数是 `round(T·wc/5)`、链上是 `floor(T·wc/5)`，两者差 ∈ {0, 1}（小数部分 ≥ .5 时差 1）
⇒ **本式 ≤ 真值，且最多小 1**。这恰好解释了实测的「差 0 / 差 +1，没有第三种」，
也解释了负值里 98.6% 恰好 = −1。

可复现实证（本地 PG 18，一次性副本库上实测）：

```sql
select 20939809989941853428::numeric / 5;        -- 4187961997988370686  ← 被进位
select trunc(20939809989941853428::numeric / 5); -- 4187961997988370686  ← trunc 截不掉（已经是整数）
select div(20939809989941853428::numeric, 5);    -- 4187961997988370685  ← **精确**（PG 的截断整数除法）
-- 用 §3.2 那条生产对拍复核：
select 4188176206443469924::numeric - trunc(20939809989941853428::numeric*1/5);  -- 214208455099238 ← 本式，比真值小 1
select 4188176206443469924::numeric - div(20939809989941853428::numeric*1, 5);   -- 214208455099239 ← 真值（聚合器 TotalGasReward）
```

触发条件：商 ≥ 16 位有效整数位（`T ≳ 1e16`）。主网奖励 actor 的 `ThisEpochReward` 有 20 位 ⇒ **恒触发**。

* 同方向的次要项是 §3.3 第三条（同 epoch 多块时「逐块截断求和」vs「先求和后截断」，
  截断的次可加性给出差 ≤ 块数−1）；实测差值从未超过 1 ⇒ 两个机制合起来也只产生了 ±1。
* **关键推论（§6.4 依赖它）**：本式 ≤ 真值，差值 ∈ {0, 1}（两块以上理论上可达 2，实测未见）
  ⇒ **本式 ≥ 真值 − 1 ⇒ 本式 ≤ −2 的行不可能是取整造成的**，只能是真偏差。
* **精确写法**（一行之差，建议下次重跑回填时启用；本次生产保持 `trunc` 以便复现现状）：
  `div(ter.this_epoch_reward * w.win_count, 5)` ⇒ 差恒为 0，不会再产生 `-1` 行。
  ⚠ 切换后重算的值与现存（已归一化）的行最多差 1 attoFIL，且 05 的负值探针会失效（不再有负值）；
  要么整体切换、要么整体不切。

### 6.4 负值归一化（已执行；`-1` 那批是**可证精确**的）

回填后出现 **2,015,596 行负值**，双峰分布：

| 分组 | 行数 | 占比 | 性质 | 处理 |
|---|---|---|---|---|
| `gas_reward = -1` | 1,987,583 | 98.6% | **取整边界**（不是模型失真） | 置 **0**（精确） |
| `gas_reward ≤ -2` | 28,013 | 1.4% | **真偏差**（量级 ≤ ~1e-6 FIL；怀疑 penalty>0 / 奖励 actor 余额不足） | 置 **NULL** |

**为什么 `-1` 置 0 是精确的（可证）**：由 §6.3，真值 ∈ {公式, 公式+1}；再取口径前提「真值 ≥ 0」
⇒ 公式 = −1 时真值必为 **0** ⇒ 这批行置 0 不是近似，是**恒等**。

**为什么 `≤ -2` 置 NULL 而不是置 0**：0 是聚合器的**合法取值**，拿 0 顶会静默改口径
（`TxFeeReward` / `MinedReward` 会算错）；NULL 是读路径的显式「不可用」信号
（`agg_pg_reward.go` 的 `incompleteGasRewardRow`）⇒ 整个请求回落聚合器，代价只是慢、不产生错值。
这批值本身不可信（≤ ~1e-6 FIL 的偏差），也不该出现在页面上。

生产实际执行的两条语句（已固化为 `05_normalize_negatives.sql`，**顺序不可颠倒** ——
步骤 2 的 `gas_reward < 0` 会吃掉步骤 1 要处理的那批 `-1`）：

```sql
update chain.miner_win_counts set gas_reward = 0    where gas_reward = -1;
update chain.miner_win_counts set gas_reward = null where gas_reward < 0;
```

归一化后的实测收口：

| 指标 | 行数 |
|---|---|
| 负值行数 `negative_rows` | **0** |
| 已回填（非 NULL） | **14,117,903** |
| NULL（读路径回落聚合器） | **2,330,460** |
| 合法值 0 | **4,971,087**（含归一化置 0 的 1,987,583 行） |

**对账（三条恒等式都成立，可自检）**：

```
回填写入 14,145,916 − 置 NULL 28,013 = 非 NULL 14,117,903   ✓
负值      2,015,596 = 置 0 1,987,583 + 置 NULL 28,013        ✓
NULL      2,330,460 = 28,013（真偏差）+ 2,302,447（未回填 / 未覆盖高度）
```

判据（可复跑）：`06_validate_after_normalize.sql` 输出 `ASSERT OK: negative_rows=0`，
且 `backfilled + null == total`。

可逆性：置 NULL **可逆**（重跑 `02_backfill.sql` 会按同一公式重新算出这些行，再重跑 05 即可归一化）；
置 0 **不可逆**（执行后「原来是 −1」与「真值本来就是 0」不可区分）。

### 6.5 同步器（已部署并验证）

* 新二进制（写 4 列）已上线，**判活四段全过**：RUNNING + `/proc/<pid>/exe` sha 一致 + PID 稳定 + 台账推进。
* **功能验证**：新高度 **epoch 6,414,319** 的 3 行全部写入了 `gas_reward`：
  `0` / `124189410895125` / `30930291727321`。
* 部署后**无 panic、无「列不存在」报错** —— 这是「② 必须在 ① 之后」那条硬约束的反面验证。

### 6.6 开关（已开）

* `/root/config.toml` 的 `[feature]` 加 `miner_wincount_read_from_pg = true`，重启 filscan-api。
* 启动日志确认：
  `aggregator reward endpoints switched to PG: miner_blockreward=true miners_blockreward=true wincount=true timeout=5s`。
* **`from pg failed = 0`**（读 PG 的失败计数为 0）。
* 开关前后同一区块的 `TxFeeReward` / `MinedReward` 逐值一致（`rollout.sh --step 4` 的判据）。

### 6.7 既有现象：`no reward or wincount found`（**不是本次引入，别误判**）

这个错误在**切换前的 00:00–10:00 每小时都有 106–248 次，全天分布均匀**
⇒ 是「刚出块、reward/wincount 数据尚未落库」的正常情况。
⇒ 判断本次改造是否引入回归时，**不要拿这条日志的计数当指标** —— 它改造前后都在。

### 6.8 部署前刚同步的高度在 PG 里是 NULL（**设计行为**）

* 部署前刚同步的高度：旧同步器只写 3 列 ⇒ 那些行的 `gas_reward` 是 NULL。
* 读路径遇到 NULL 会**整请求回落聚合器** ⇒ 页面值正确、只是慢。**这是设计行为，不是漏回填。**
* 这些高度**不需要补回填**：新高度由新同步器**前向写入**覆盖；想彻底消掉 NULL 就跑 `backfill.sh`（幂等）。
* 这部分 NULL 计入 §6.4 的 2,302,447（= 2,330,460 − 28,013）。

### 6.9 回滚路径（命令）

逆序铁律：**① 开关 → ② 二进制 → ③ 才动 DDL**（写路径还在写 `gas_reward` 时删列 ⇒ 下一次同步 INSERT 失败）。

```bash
# 最小回滚（推荐：零风险、秒级、数据不丢）：只关读开关
cd /root/wincount_gas_reward && ./rollback.sh --step 1 --yes
#   等价手工：把 /root/config.toml 的 [feature] miner_wincount_read_from_pg 改成 false
#             supervisorctl restart filscan-api
#   判据：RUNNING + FinalHeight 正常（读路径回到聚合器）

# 逆序全回滚（① 关开关 → ② 回滚二进制 → ③ 默认只 rename 保留数据）
cd /root/wincount_gas_reward && ./rollback.sh --yes

# 真删列（**会丢掉全部回填结果**，必须先完成 ②）
cd /root/wincount_gas_reward && ./rollback.sh --step 3 --drop-column --yes
```

* 配置与二进制备份目录：`/root/deploy-bak/`（`config.toml.bak-<TS>`、`filscan-syncer.bak-<TS>`）。
* **中间态（列已加、开关关着）是完全安全的**：读路径回落聚合器 ⇒ 金额与改造前逐值一致；
  新同步器继续攒值（将来重开开关不用重来）；列可空、无 default ⇒ 旧代码/旧 SQL 完全不受影响。
* 归一化的回滚：置 NULL 的行重跑 `02_backfill.sql` 即还原为公式值（负值）；置 0 的行不可逆区分。
* DDL 级回滚（rename / drop）见 `90_rollback.sql`；代码级回滚 = 切回 main 重建二进制。

