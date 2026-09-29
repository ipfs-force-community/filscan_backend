# agg-parity：聚合器 vs PostgreSQL 两路径一致性校验

**用途**：在把某个端点的读路径从「聚合器（打冷库）」切到「读 PostgreSQL 表」之前和之后，
逐行比对两条路径的结果，回答一个问题——**现在能不能切？**

## 怎么跑

在**既能连 PG、又能连聚合器**的机器上跑（例如 backend 主机），配置用现成的 `config.toml`：

```bash
# 只跑 PG 侧自检（不调聚合器，最常用的第一步）
./agg-parity -c /root/config.toml -endpoints large_amount -index 0 -limit 20 -pg-only

# 两侧完整比对。该端点线上极慢（实测 60 秒零字节挂住），务必给足超时
./agg-parity -c /root/config.toml -endpoints large_amount -index 0 -limit 20 -agg-timeout 5m

# 想一起看 PG 侧 SQL（与线上管线逐条对照）
./agg-parity -c /root/config.toml -endpoints large_amount -index 0 -limit 20 -pg-only -print-sql
```

`-index/-limit` 默认 `0/0`＝线上语义「取全量」（会拉两次全表），**对生产务必显式给 `-limit`**。
大额转账端点不需要 `-start/-end`（线上该请求不带高度区间）；带区间的端点仍强制要求。

## 切开关前的必做清单（顺序不能颠倒）

1. **先补齐增量缺口**（关键）：把热库水位之后的行补进表，否则第一页会落后最新数据。
   跳板机：`bash /root/refresh_hot.sh`（幂等；等于「同区间先删后插」，可重复跑）。
2. **`-pg-only` 自检**：期望 `PASS`、行数 = 请求的 `limit`、`TotalCount` = 表当前行数。
3. **多页抽样**：`-index` 取第一页 / 中间页 / 末页 / 越界空页（越界应两侧皆空、不报错）。
4. **完整比对**：`-agg-timeout 5m`。若聚合器不可达/挂死，用 `-pg-only` 拿到 PG 侧证据，
   并**在切换记录里写明「聚合器侧无法取证」**，不要默认它是通过的。
5. **再打开开关**：`config.toml` 的 `[feature]` 段加 `large_amount_read_from_pg = true`，重启
   `filscan-api`（挑低峰，重启会短暂中断前台服务）。
6. **观察**：日志里出现 PG 回落 `Warn` 就说明 PG 侧不稳，此时**先关开关再排查**，不要让前端持续吃回落。
7. **切换后复跑一次**本工具（两侧比对），确认线上返回与 PG 仍一致。

## 判定规则

| 报告项 | 含义 | 处理 |
|---|---|---|
| `[数值不等]` | 同一行金额/深度等数值不同 | **禁止打开开关**，先查数据装载 |
| 行缺失 / PG 独有行 | 行集合不一致 | **禁止打开开关** |
| `[仅文本不同]`（地址形态，如 `5eeee…` vs `f5eeee…`） | 两端地址前缀形态不同 | 确认前端消费形态后决定；`-strict-format=false` 可放宽 |
| `order_diff`（同高度内行序） | 线上同高度内行序**未定义**，PG 用 `epoch desc, cid asc` 定序 | **已知差异**，默认不判失败；`-strict-order` 可升级为失败 |
| `TotalCount` 不等 | 线上计数来自另一条管线（可能陈旧） | 先用 `-pg-only` 核全表行数，确认后可用 `-lenient-total` |

## 回滚

开关是**默认关闭**的普通配置项：把 `large_amount_read_from_pg` 改成 `false`（或删掉该行）并重启
`filscan-api` 即回到原行为，**不需要回滚数据**。PG 报错/超时/表缺失时会自动回落聚合器并打 `Warn`，
前端不会因此 500。

## 口径（与线上管线逐条对齐，改这条路径前必读）

- 过滤条件：`MsgRct.ExitCode = 0` 且 `FIL >= 10000`（等价于 `Msg.Value >= 1e22` attoFIL）。
- **不去重**：线上 `count` 是 `$group{$sum:1}` 数 trace 行，列表管线**没有 `$group`**；
  同一高度同一 cid 可能多行（同高度竞争区块各自留一行），分片区间边界重叠时相邻两库会各返回一份。
  ⇒ 表**没有主键、禁止 `distinct`/`group by`**，`TotalCount` 是**全表 `count(*)`**（不按区间过滤）。
- 分页：`index` 是**页码**，`offset = index * limit`；`index <= 0 且 limit <= 0` 时线上取全量。
- 定序：PG 侧 `order by epoch desc, cid asc`（线上同高度内未定义，见上面的已知差异）。
- 区间语义：左闭右开。
