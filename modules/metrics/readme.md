# modules/metrics —— 同步落后高度 / 关键表新鲜度指标

本包由 `cmd/metrics`（二进制名 `filscan-metrics`）驱动，产出**可判、口径唯一**的落后量指标。

## 为什么存在

2026-09 主网索引链静默停摆 18 天而**没有任何有效告警**：

| 现象 | 实测 |
|---|---|
| 旧指标 `filscan_syncer_delay_height` | `chain=2` / `miner=60`（口径坏，永不触发阈值 720） |
| 真实落后 | **51393 / 51347** 个高度（18 天） |
| 四条规则引用的指标 | `pushgateway_filscan_check_height_sync` / `pushgateway_filscan_check_syncer_log` / `pushgateway_filscan_logfile_update` / `londobell_outdated_final_height` 在 Prometheus 里**根本不存在**（死规则） |

结论：告警必须落在**「链头高度 − 表/游标已处理高度」**这一层，而不是任何中间过程的差值。

## 指标语义（单位一律为「链高度 / epoch」）

| 指标 | 标签 | 含义 | 取数 |
|---|---|---|---|
| `filscan_chain_head_height` | `network` | 链头高度 | londobell adapter `POST /adapter/epoch`（body `{"epoch":null}`） |
| `filscan_chain_head_block_time_seconds` | `network` | 链头区块时间（Unix 秒） | 同上 `data.block_time` |
| `filscan_syncer_cursor_height` | `syncer`, `network` | 该同步器**已处理（落库）**高度 | `select "name", "epoch" from "chain"."sync_syncers"`（同名多行取最大） |
| `filscan_syncer_lag_height` | `syncer`, `network` | **链头高度 − 该同步器已处理高度** | 上两行相减 |
| `filscan_table_cursor_height` | `table`, `column`, `network` | 该表**停在多少高度** | `select max("<column>") from "<schema>"."<table>"` |
| `filscan_table_lag_height` | `table`, `column`, `network` | **链头高度 − 该表 max(高度列)** | 上两行相减 |
| `filscan_metrics_up` | `network` | 1=取到链头；0=链头不可用（此时**不产出**任何 lag 指标） | — |
| `filscan_metrics_collect_cycles_total` | `network` | 采集轮数（确认采集进程活着） | — |
| `filscan_metrics_collect_errors_total` | `scope`, `table`, `reason`, `network` | 单点失败累计（`reason`∈`table_missing`/`empty`/`timeout`/`query_failed`） | — |
| `filscan_syncer_delay_height` | `syncer`, `network` | **DEPRECATED 兼容名**：与 `filscan_syncer_lag_height` 同源同口径同值 | 同上 |

`table` 标签取 `schema.table`（如 `fevm.evm_transfers`），`column` 取高度列名（如 `epoch`）。

### 新旧关系（重要）

`filscan_syncer_delay_height` 是**兼容名**，值等于 `filscan_syncer_lag_height`：
只要规则指向本采集器，旧名字也能拿到正确口径；可用 `-no-legacy` 关掉它。
**必须避免**旧 pushgateway 推送脚本（口径坏的那个）与本采集器同时喂同一个指标名——
两个数据源会产生两条同名序列，规则聚合（如 `max()`）会取到坏的那条，等于没修。迁移顺序：先把规则切到本采集器 → 再停旧推送脚本。

## 关键表清单（默认，可用 `-table` 或 `[metrics].tables` 覆写）

```
chain.sync_syncers:epoch        # 同步器游标表（最关键：任一同步器停摆立即显形）
chain.sync_syncer_epochs:epoch  # 逐高度执行留痕
chain.actor_actions:epoch       # actor syncer
chain.rich_actors:epoch         # chain syncer（账户余额）
chain.miner_infos:epoch         # miner syncer
pro.miner_sectors:epoch         # sector syncer
fevm.evm_transfers:epoch        # evm syncer
fevm.erc_20_transfers:epoch     # erc20 syncer
fns.transfers:epoch             # fns syncer
```

## 运行

```bash
# 编译
make build-metrics              # → ./bin/metrics

# 拉模式（Prometheus 直接抓）
./bin/metrics -c /etc/filscan/config.toml -listen 127.0.0.1:10020
#   GET /metrics  Prometheus 文本格式（TTL 15s 缓存，不会每抓一次打一次库）
#   GET /healthz

# 推模式（交给现有 pushgateway 链路）
./bin/metrics -c /etc/filscan/config.toml -once \
  | curl --data-binary @- http://pushgateway:9091/metrics/job/filscan_metrics
```

### 配置（整节可省略）

```toml
[metrics]
address           = "127.0.0.1:10020"   # 监听地址
interval          = 15                  # 采集/渲染缓存间隔（秒）
query_timeout     = 15                  # 单条 SQL / 链头接口超时（秒）
legacy_delay_name = true                # 是否输出兼容旧名 filscan_syncer_delay_height
tables            = ["chain.actor_actions:epoch", "fevm.evm_transfers:epoch"]
```

## 建议的告警规则（供监控栈落地，本包不负责 Prometheus 配置）

```promql
# 任一同步器落后超过 720 个高度（≈6 小时）持续 10 分钟
max by (syncer) (filscan_syncer_lag_height) > 720

# 表新鲜度：链头 − 表最高高度（直接回答「哪张表停在多少高度」）
max by (table) (filscan_table_lag_height) > 100

# 采集器自身挂了（链头取不到 / 进程没了）
filscan_metrics_up == 0 or absent(filscan_metrics_up)
```

## 容错约定

- 某张表不存在 / 权限不足 / 超时：只累加 `filscan_metrics_collect_errors_total`，该表系列缺失，**其余指标照常产出**；
- 链头取不到：`filscan_metrics_up=0` 且**不产出**任何 lag（宁缺毋滥：缺数据可以用 `absent()` 告警，算错的 lag 会让规则永久失效）；
- 单条查询有 15s 超时（可配），采集间隔默认 15s：监控不得把业务库拖慢。
