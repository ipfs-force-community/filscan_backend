# 上线件交付说明（wincount gas_reward）

> ⚠ **本文件记录的是「上线前的暂存状态」（历史快照）。** 实际执行结果、实测数字与结论见
> `README.md` 的 **§6「上线实录（2026-09-30）」** —— 迁移/回填/归一化/开关**当日已在生产跑完**，
> 下面「本次交付未执行任何生产改动」只描述这份交付件当时的边界，**不代表分支当前状态**。
> 另外：`04_*.sql` / `dryrun_premigration.sh` / `rollout.sh` / `rollback.sh` 已随本文件一并纳入版本管理。

> 分支 `feat/wincount-gas-reward @ 8c079e2`　本次新增的是**上线件**，不改业务代码。
> 目标机：backend `172.31.34.109`（`ssh -o ProxyJump=filscan-jumpserver root@172.31.34.109`）。
> **本交付件编写时未执行任何生产改动**：没跑 DDL、没装二进制、没回填、没改 config。

## 0. 新增/更新的文件

| 文件 | 作用 | 状态 |
|---|---|---|
| `rollout.sh` | 上线编排（① migration → ② 二进制 → ③ 回填 → ④ 开关），每步独立判据 + 失败即停 + ② 自动回滚 | **新** |
| `rollback.sh` | 逆序回滚（① 关开关 → ② 回滚二进制 → ③ 才动 DDL），默认 dry-run | **新** |
| `04_dryrun_premigration.sql` | **DDL 之前**的影响面估算（`02_backfill.sql` 同口径，去掉 `gas_reward is null`，等价性已在文件头证明） | **新** |
| `dryrun_premigration.sh` | 按分区跑 `04_*.sql`，输出「多少分区 / 多少行 / 耗时」 | **新** |
| `01_preflight.sql` / `02_backfill.sql` / `03_validate.sql` / `90_rollback.sql` / `backfill.sh` / `README.md` | 原有 | 未改 |
| `migration/36.miner_win_counts_gas_reward.sql` | 原有 | 未改 |

## 1. 二进制（已构建 + 过指纹门 + 已暂存到目标机）

- 本地：`/Users/elvindu/filscan-repo/artifacts/filscan-syncer-wincount`
- 目标机暂存（**未安装**，`/root/filscan-syncer` 仍是现役 `76ce4e25…`）：
  `/root/wincount_gas_reward/filscan-syncer-wincount`
- `sha256 = 71ccc37710da43ef8bd386964e4b51ab43491da538d17e8e6fa24563f5374dc0`，`bytes = 75846218`
- 构建命令（**不要用 `make build-syncer`**：会跑 wire 生成、改写 go.mod/go.sum）：
  ```bash
  # 先补生成被 .gitignore 掉的 bundle 文件（make bundle 做的事，不动 go.mod）
  go run cmd/gen-opengate-bundle/main.go
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build --tags=bundle \
    -ldflags "-X main.Version=mainnet_8c079e2 -X main.Name=filscan-syncer" \
    -o /tmp/filscan-syncer-wincount ./cmd/syncer
  ```
- 指纹门结果见 `../../artifacts/fingerprint-report.txt`：`IRegistrar=6`、`state_gap_jump=2`、`calibnet=0`、
  版本串 `mainnet_8c079e2` 在位；本分支新增逻辑用 **A/B 对照**验证（同命令在 base `51b5985` 上构建）：
  `toMinerWinCount` 1 vs 0、`GasReward` 7 vs 6。
  ⚠ `strings | grep -c gas_reward` **两版都是 0**：列名字面量只在 PG 读路径（`cmd/syncer` 不可达 ⇒ 被死代码消除），
  不是构建问题。详见指纹报告里的说明。

## 2. 只读体检与 dry-run 结果（原样带回）

- 体检输出：`../../artifacts/preflight-output.txt` —— `relkind=p`、`partitions=317`、`has_gas_reward_col=0`、
  两条 `range_insert_action_rule` 在位、最新分区 `w40_…_6407280_6427440`、`est_rows=16046889`、
  `f02@6330000 → ThisEpochReward=20939809989941853428`。
- 回填 dry-run：`../../artifacts/dryrun-backfill-output.txt` —— **317/317 个分区，其中 147 个有待更新行，
  预计影响 14,140,489 行，只读扫描耗时 89s**。
  ⚠ 这个数字比 `README.md` 的「每分区约 5 万行」大一个量级（2024 年以后的分区每个约 9.8 万行）。
  正式回填是 14.1M 行的 UPDATE，且每个分区带 3 个 btree 索引（`(epoch,miner)` / `(miner,epoch)` / `(epoch)`，
  `gas_reward` 非索引列但默认 fillfactor=100 ⇒ HOT 多半不生效、要连带更新索引）⇒
  **务必用 `--max-partitions N --sleep S` 分批**，不要一把梭；预留的时间/IO 预算按 14.1M 行算。

## 3. 怎么跑（都在目标机上、都在 `/root/wincount_gas_reward/`）

```bash
# 全程默认 dry-run；确认后加 --yes
./rollout.sh --step 1                                  # ① 只跑 migration + 后检
./rollout.sh --step 2 --binary /root/wincount_gas_reward/filscan-syncer-wincount \
             --expected-sha 71ccc37710da43ef8bd386964e4b51ab43491da538d17e8e6fa24563f5374dc0
./rollout.sh --step 3 --max-partitions 4 --sleep 5     # ③ 分批回填（可中断、可续跑）
./rollout.sh --step 4                                  # ④ 开开关（含开关前后金额比对）

./rollback.sh --step 1                                 # 最小回滚（推荐）：只关开关
./rollback.sh                                          # 逆序全走（③ 默认只 rename）
```

**顺序不能倒的硬约束**（细节见 `../../artifacts/HARD-ORDERING-CONSTRAINT.md`）：
新同步器写 4 列，**列不存在时 INSERT 直接失败** ⇒ ① 必须早于 ②；回滚则反之，**写路径未回滚前绝不能删列**。
