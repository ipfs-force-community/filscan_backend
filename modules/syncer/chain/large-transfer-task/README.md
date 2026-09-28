# 大额转账增量维护（chain.large_transfers）交接说明

分支：`feat/large-transfers-pg-maintain`（基线 `bb20274`）· 补丁**未提交**，等合入方验收后自行 commit/merge。

## 一句话

同步器每处理一个高度，把该高度新出现的「大额转账」按**同一事务内先 delete 后 insert** 的语义写进
`chain.large_transfers`；默认开关关闭（关闭时一个 SQL 都不发），复用同步器**已有的** traces 获取链路，
不新增任何聚合器请求。

## 为什么需要它

- 线上 `/aggregators/transfer_message_for_largeAmount` 每次请求都去冷库（11 个 mongo 分片）跑
  `transfer_message_for_large_amount.js`：命中面极小，但要靠 `FIL` 索引扫冷库再回表取文档，实测单请求
  60 秒零字节（打不通）；同一次请求还要在 12 个库上各重算一遍命中行数。
- 已由父会话把**全链历史**一次性抽取进 `chain.large_transfers`（245,116 行，快照水位 `max(epoch)=6409152`）。
- 缺的是**增量**：热库那段数据会过期（实测一小时 +6 行）。本补丁就是这一步，让读取侧以后只扫一张小表。

## 开关

| 项 | 值 |
|---|---|
| 配置项 | `[syncer] sync_large_transfers` |
| 默认 | **false**（字段缺失/null 也是 false，老配置文件不会 panic） |
| 关闭时行为 | 同步器**根本不注册**该任务：零 SQL、零 `chain.sync_task_epochs` 行、行为与打补丁前完全一致 |
| 访问器 | `config.Syncer.SyncLargeTransfersValue()`（`modules/common/config/config.go`） |

理由：先合代码、确认无副作用，再单独开开关。任务内部还会再判一次开关（双保险，防将来有别的装配路径
无条件注册它）。

## 插入点与理由

挂载点：`injector.NewSyncerManager` 里 **chain 同步器（`syncer.ChainSyncer`）任务组的第一位**
（`injector/syncer_manager.go:160-180`）。

1. 这个同步器已经注册了 `injector.SetTracesBuilder`（`syncer_manager.go:45`），它每个高度都会调
   `ctx.Agg().Traces(ctx, epoch, epoch.Next())`，把**该高度的 traces 原始文档**放进 Datamap。
   本任务只读 Datamap 里现成的这份数据 ⇒ **不新增任何聚合器请求**（新建同步器、或另起一次按高度取
   traces，都会让每高度成本翻倍，明确不做）。fevm 各子同步器与离线回放走的是同一条链路。
2. 放在组内**最前**：本任务只依赖 traces。若排在其后，前面任一任务报错会让整组提前返回，该高度的增量
   就漏了；反过来本任务自身**从不返回错误**，所以不会拖累后面的任务。
3. 关闭时不注册 ⇒ 一致性检查（`Syncer.CheckConsistency` 用**已注册**任务列表算任务覆盖度）不会因为
   少一个任务而误判回滚；开启/关闭来回切也安全。

## 改了什么（文件清单）

| 文件 | 位置 | 改了什么 |
|---|---|---|
| `modules/syncer/chain/large-transfer-task/large_transfer_task.go` | 新增，459 行 | 任务本体：筛选（`SelectLargeTransfers`）、写库、幂等与失败策略、`RollBack` |
| `modules/syncer/chain/large-transfer-task/large_transfer_task_test.go` | 新增，612 行 | 单测（假仓储，不连库） |
| `modules/common/infra/po/chain_large_transfer.go` | 新增，36 行 | PO：`po.LargeTransfer`，`TableName() = "chain.large_transfers"`，列名与 DDL 一致 |
| `modules/common/infra/dal/dal_large_transfer.go` | 新增，64 行 | DAL：**同一事务内** delete-then-insert + `DeleteLargeTransfersFromEpoch` |
| `modules/common/infra/dal/dal_large_transfer_test.go` | 新增，296 行 | DAL 单测（假连接：断言一个事务、语句顺序、参数逐列逐值） |
| `modules/common/repository/repository.go` | 359-374 | 新增 `LargeTransferRepo` 接口（写入侧） |
| `modules/common/config/config.go` | 120-135 | 新增 `SyncLargeTransfers *bool` + `SyncLargeTransfersValue()` |
| `modules/syncer/context.go` | 22-37 | 新增 `NewTestContextWithData`（**仅供单测**：构造带 Datamap 的 Context，与既有 `NewTestContext` 同一用途） |
| `pkg/londobell/agg.go` | 673-688 | `Msg` 结构补两个字段：`Value *decimal.Decimal`、`MethodName string` |
| `injector/syncer_manager.go` | 26、160-180 | 装配：开关打开时把任务插到 chain 任务组最前 |

`pkg/londobell/agg.go` 那两个字段是**补解码**而非新语义：`/aggregators/traces` 的管道本来就把整份 `Msg`
投影出来（`traces.js:43` 的 `Msg: "$Msg"`），线上大额转账管道用的正是 `$Msg.Value` / `$Msg.MethodName`，
此前只是没被 Go 结构体解码。其他任务（deal-proposal 等）只读 `Msg.Method`，补字段不影响它们。

## 过滤判据：为什么用 `Msg.Value >= 1e22` 而不是 `FIL`

线上管道（`londobell-aggregators/pool-monitor/transfer_message_for_large_amount.js:149-156`，文件里**在跑**的那段）：

```js
$match: { "MsgRct.ExitCode": 0, "Epoch": { $gte: ctx.StartEpoch, $lt: ctx.EndEpoch }, "FIL": { $gte: 10000 } }
```

- `FIL` **拿不到**：traces 响应由 `traces.js` 的 `$project` 决定，其中没有 `FIL`
  （只有 `ID/Cid/SignedCid/Epoch/Seq/Depth/Ver/Msg/MsgRct/Error/SeqIndex/SubCallCount/GasCost/ReturnBson/Version/To/From/Nonce/Value/IsBlock/…`）。
  为了不新增一次请求，改用 `Value` 判定。
- **严格等价的证明**：`FIL` 是写入 mongo 时算好的整型字段
  `me.FIL = CalculateFILValue(me.Msg.Value.String())`（londobell `racailum/segment/model/exec_trace.go:80/273`），
  而 `CalculateFILValue` 就是「取十进制的整数部分」= `floor(V / 1e18)`。于是对非负整数 attoFIL 值 `V`：

  ```
  FIL >= 10000  ⟺  floor(V/1e18) >= 10000  ⟺  V >= 10000 * 1e18 = 1e22
  ```

  边界：`10000000000000000000000`（=1e22）命中；`9999999999999999999999`（=1e22−1）不命中。
  单测两个边界都覆盖。顺带一提，同一文件里被注释掉的旧写法 `{ $regexMatch: { input: "$Value", regex: "^.{23,}$" } }`
  （“至少 23 位数字”= ≥1e22）对规范整数文本与上面**同界**。
- 比较全程 `shopspring/decimal`，写入是十进制字符串，**绝不经过浮点**。
- `"MsgRct.ExitCode": 0` 的 mongo 语义是「`MsgRct` 存在且 `ExitCode==0`」——缺 `MsgRct` 的文档**不匹配**，
  不是按 0 处理。实现里用 `MsgRct != nil && ExitCode == 0` 复刻（有单测）。
- 金额取 `$Msg.Value`（线上 `value` 列也来自它）；`Msg.Value` 缺失才退回一并返回的 `message.Value`。

## 列口径（与线上 `$project` 逐列对齐）

| 列 | 来源 | 说明 |
|---|---|---|
| `epoch` | `ctx.Epoch()` | 与文档 `Epoch` 相同：traces 就是按 `[epoch, epoch+1)` 取的；而且是 delete 的键，必须用同一值 |
| `cid` | `SignedCid ?? Cid` | 空串也回退（不写空 cid） |
| `root_cid` | 见下 | 可空 |
| `from_addr` / `to_addr` | `$Msg.From` / `$Msg.To` | 落库形态是 mongo 里那种**不带网络前缀**的原文（`addressBSONEncode = addr.String()[1:]`） |
| `value` | `$Msg.Value` | attoFIL 十进制字符串，不经过浮点 |
| `method` | `$Msg.MethodName` | 缺失才退回 `Detail.Method` |
| `depth` | `$Depth` | |

**`root_cid` 的还原**（traces 响应里没有 `RootCid`/`RootSignedCid` 字段，不新增请求就只能自己算）：
线上取值是 `RootSignedCid ?? RootCid`，而这两个字段是写入 mongo 时按聚合器自己的算法填的
（londobell `racailum/segment/model/exec_trace.go`）：

```go
if met.IsBlock { IDCidMap[met.ID] = [2]cid.Cid{met.Cid, met.SignedCid} }   // 只有块级消息登记
et.RootCid, et.RootSignedCid = m[rootID][0], m[rootID][1]                  // 子调用查「Epoch + Seq 首元素」
// 且 IsBlock 的文档直接 return —— 根消息自己两个字段都不写 ⇒ 线上投影得到 null
```

`ExecTrace._id = "<Epoch>-<Seq 各段补零用 - 连接>"`，`GetRootID` 取的就是「Epoch + Seq 首元素」。
值得一提的是 `Seq` 首元素是 **tipset 级**消息序号（`for i := range invocs { seq: []int{i} }`，
覆盖该高度所有区块的消息），所以在同一高度内唯一 —— 这也是 `_id` 能用 Epoch+Seq 拼出唯一键的原因，
按它做索引不会串到别的消息树（包括「同一高度同一 cid 出现两次」的竞争区块场景，实测有 24 组，
它们的 `Seq` 各不相同，各自解各自的根）。
所以本实现按同一规则还原：只有 `IsBlock==true` 的 trace 才是根候选（索引键 = Seq 首元素），
子调用（`Seq` 多于 1 段）取该根的 `SignedCid ?? Cid`；**根消息自己与找不到根时写 NULL**。
这与一次性抽取写下的历史行口径一致（历史行的 `root_cid` 同样是 NULL 或根消息 cid）。

## 幂等与失败策略（实现位置）

| 要求 | 实现 |
|---|---|
| 幂等 | `dal_large_transfer.go: ReplaceLargeTransfers` —— **一个 gorm 事务**内 `delete from chain.large_transfers where epoch = $1` 再 `CreateInBatches`；没有主键/唯一索引，不做 upsert，靠「重写就一致」 |
| 重复处理结果一致 | 单测 `TestExecIsIdempotentOnReplay`（同一高度跑 4 次，全表快照逐次相等）+ DAL 单测断言 `txBegins/commits/rollbacks == 1/1/0` |
| 失败不阻断 | `large_transfer_task.go: Exec` 写失败只 `Errorf` + 计数（`FailedEpochs()`），**返回 nil**；用**独立事务/独立 context**（`context.WithTimeout(context.Background(), 15s)`），不是 `ctx.Context()` |
| 为什么不复用 `ctx.Context()` | `syncer.execTaskOrCalculator` 把该高度的 PG 事务挂在 `ctx.Context()` 里（`syncer.go:1112`）：写失败会让那个事务进入 aborted，随后 `Commit` 必报错 ⇒ 该高度被判失败并无限重试。必须用独立事务 |
| 表不存在 | 只 Warn **一次**（`MissingTableWarns()` 计数，之后降级 Debug），不刷屏也不静默 |
| Datamap 没有 traces | 只 Warn 一次（装配问题），不返回错误（否则该高度无限重试） |
| panic 兜底 | `Exec` 有 recover：畸形 trace 变成一条 Error 日志，不影响本高度其余任务 |
| 回滚 | `RollBack` 删 `>= gteEpoch` 的行；**失败不上抛**（上抛会让 `syncer.Rollback` 失败，同步器卡在回滚-重试循环），靠这些高度重跑覆盖 |
| 没有 traces 时不删 | `traces` 为空/缺失时**不下发任何 SQL**：空 traces 无法证明「该高度本来没有大额转账」，误删会抹掉回填的历史行；链重组变空由 `RollBack` 覆盖 |

## 合入与上线

基线是 `bb20274`（另一条工作线的热修分支），可直接 `merge` 或 `cherry-pick`；改动集中在 10 个文件，
不与 `modules/syncer/data_error.go`（跳过台账/错误分类）有任何交集——那套一个字没动。

```bash
# 1. 合入后先只编译与跑门禁（开关仍关闭）
go build ./...
go test ./modules/syncer/...
# 2. 建表（DDL 在另一条工作线，本分支工作树里没有这个文件：commit 134efd7 的
#    migration/35.large_transfers.sql；本补丁不改 DDL，只往表里写数据）
#    psql -f migration/35.large_transfers.sql
# 3. 打开开关（同步器 toml）
#    [syncer]
#    sync_large_transfers = true
# 4. 重启同步器，看日志
```

表结构（已存在，本补丁不改）：`chain.large_transfers(epoch bigint, cid text, root_cid text,
from_addr text, to_addr text, value numeric(38,0), method text, depth integer)`，
只有 `(epoch)` 一个普通索引，**无主键、无唯一索引**（线上口径要保留 trace 行重数）。

验收（把高度换成目标值）：

```sql
-- ① 某高度是否写入（应为该高度命中行数）
select count(*) from chain.large_transfers where epoch = :epoch;

-- ② 明细，逐列核对（epoch/cid/root_cid/from_addr/to_addr/value/method/depth）
select epoch, cid, root_cid, from_addr, to_addr, value, method, depth
from chain.large_transfers where epoch = :epoch order by depth, cid;

-- ③ 增量是否在推进（开启后每隔几分钟跑一次，最新高度应随链前进）
select epoch, count(*) from chain.large_transfers group by epoch order by epoch desc limit 10;

-- ④ 与快照水位的衔接：开启前的缺口有没有被回填补上
select max(epoch) from chain.large_transfers;   -- 一次抽取时是 6409152

-- ⑤ 幂等自检：重跑某高度前后，该高度的行数与内容不变
select epoch, count(*) from chain.large_transfers where epoch between :a and :b group by epoch order by epoch;
```

日志判据（`chain` 同步器）：

- 写入：`高度 <epoch> 大额转账增量已写库: N 行（同一事务内 delete-then-insert）`
- 写失败（不阻断）：`高度 <epoch> 大额转账增量写库失败（累计第 N 个失败高度）: ... 该高度的增量需重跑补齐`
- 表不存在：`... 目标表 chain.large_transfers 不存在（第 1 次失败）...`（只出现一次）
- 装配问题：`... Datamap 里没有 traces（本同步器是否忘了 WithContextBuilder(SetTracesBuilder)？）...`（只出现一次）

**⚠️ 开启后不会自动回补历史**：同步器只处理它当前推进到的高度，`execTaskOrCalculator` 跳过的判断是
「该高度该任务已有台账行」。所以**从快照水位 6409152 到开启那一刻之间的高度**要靠回填/replay 覆盖
（父会话的回填战役本来就跑在这段上）。开启前也可以先把 `chain.sync_syncers.epoch` 改回区间起点重跑，
本任务的 delete-then-insert 语义对重跑是安全的。

## 回滚口径

1. **只想停**：把 `sync_large_transfers` 改回 `false` 重启 —— 一个 SQL 都不再发，已写入的数据保留。
2. **数据回滚**（链回滚或整段重跑导致的）：任务自带 `RollBack`，按 `>= 回滚高度` 删除；手工等价操作是
   `delete from chain.large_transfers where epoch >= :rollback_epoch;`，之后重跑这些高度会按同一语义重写。
3. **整补丁回滚**：`git revert` / 还原 10 个文件即可，无副作用（这张表只有这一个写入口）。

## 已知差异（如实登记）

1. **分片边界重叠高度**：相邻两个冷库在边界上同时持有一份相同文档（实测 262 个 `(epoch,cid)`），线上会把
   它们算两次，一次性抽取已按此重数写入。回放若覆盖到这些旧高度，写入的行数取决于 traces 来源是否合并了
   两个库（链式取数只走一个库 ⇒ 可能只写一条，少的是「重叠重数」而不是丢数据）。这类高度**只有回放会碰**；
   本补丁的 delete-then-insert 语义保证「重写一次就一致」，同一来源重复回放结果稳定。
2. **回补窗口**：见上（快照水位 → 开启时刻）。本补丁不做补偿任务，也不改同步器的推进/跳过逻辑。
3. `Msg.Value` 缺失时才退回 `message.Value`；线上管道在 `$Msg.Value` 缺失时会得到 null 值——这类畸形文档
   极罕见，且退回的是同一条消息的同一金额（更保守，不会漏）。同理 `method` 退回 `Detail.Method`。
4. 地址优先取 `$Msg.From`/`$Msg.To`，为空才退回 trace 自己的 `From`/`To`（同一条消息的同一地址）。
   两种输入形态都兼容：带网络前缀（`f1…`/`t3…`）去掉首字符；已经是原文则原样写出
   （不能直接调 `SmartAddress.CrudeAddress()`：它对**已无前缀**的输入会返回空串，会把地址写成 `''`）。
5. 地址字段彻底缺失时写 `''`（线上投影缺字段会是 NULL）——极罕见，仅畸形文档。
6. 本表按行保留重数（与线上「列表无 `$group`」一致；计数侧线上是 `$group{$sum:1}` 数行 ⇒ 一致）。
   `chain.large_transfers` 没有主键、也没有唯一索引（有意为之）。

## 本次已做的本地验证（门禁输出）

```
$ go build ./...
（通过；只有既有的 ld: warning: ignoring duplicate libraries: '-lhwloc' 警告）

$ go test ./modules/syncer/...            # 失败集合与本补丁无关，见下
ok   .../modules/syncer/chain/large-transfer-task   0.465s
FAIL .../modules/syncer                               (TestChainSyncer, TestMinerSyncer, ...)
FAIL .../modules/syncer/calculator/calc-change-actor-task
FAIL .../modules/syncer/calculator/calc-miner-owner-task/luck
FAIL .../modules/syncer/fevm/erc20
FAIL .../modules/syncer/fevm/evm_transaction/evm_test
FAIL .../modules/syncer/fevm/nft

# 与基线对比（同一台机、同一 module cache，直接在 bb20274 的干净 worktree 里跑同一命令）：
#   baseline 失败用例 15 个 / 6 个包  ←→  打补丁后失败用例 15 个 / 6 个包
#   diff 结果：失败集合完全相同（唯一差异是 TestIsPairFunc 的耗时 1.11s/0.81s）
#   即：本补丁**没有让既存失败集合变大**（那批失败都是缺 configs/*.toml 等环境依赖）

$ go test ./modules/common/infra/dal/ -run LargeTransfer -v
--- PASS: TestReplaceLargeTransfersDeleteThenInsertInOneTx
--- PASS: TestReplaceLargeTransfersWithoutRowsStillDeletes
--- PASS: TestReplaceLargeTransfersRollsBackOnFailure (insert 失败 / delete 失败)
--- PASS: TestDeleteLargeTransfersFromEpoch
```

新增单测清单（全部离线：假仓储 + 假 gorm 连接，不连库、不连聚合器）：

- 筛选边界：`1e22` 命中、`1e22−1` 不命中、`1e22+1` 命中、1 FIL/0 不命中、`ExitCode≠0` 不命中、
  `MsgRct` 缺失不命中、`nil` trace 不命中、`Msg.Value` 优先（与 message.Value 不一致时）、
  `Msg.Value` 缺失退回、`Msg` 缺失不 panic。
- 列映射：`SignedCid` 优先/空串回退、`root_cid` 的四种情形（根消息⇒NULL、非 IsBlock 单段⇒NULL、
  子调用找到根⇒根的 SignedCid??Cid、找不到根⇒NULL）、`method` 优先/回退/空串、
  地址三态（带前缀⇒去首字符、已原文⇒原样、`Msg` 空⇒退回 trace）、同一 cid 多行保留。
- 幂等：同一高度跑 4 次全表快照一致。
- 失败不阻断：写失败返回 nil + 计数 + 不落半截数据，之后的高度照常写入。
- 表不存在只 Warn 一次（3 次失败 ⇒ 告警计数 1）。
- 开关关闭零 SQL（`Exec` 与 `RollBack` 都不调仓储）。
- 无 traces/空 traces/类型不对：零 SQL（只 Warn 一次）。
- DAL：一个事务、语句顺序（delete 在 insert 之前）、参数逐列逐值（含 `value` 的十进制原文、`root_cid` 的
  NULL、地址原文）、失败回滚、无命中行时只 delete。
- 配置开关默认 false（`nil` 段/未配置 → false）。

## 需要父会话/合入方实测确认的点（我拿不到生产环境，只能静态对齐）

1. **traces 响应里地址的实际形态**（是否带 `f`/`t` 前缀）：两种形态都已兼容并各有单测，但建议开启后抽
   一条新高度的行与一次性抽取的同 cid 老行对比，确认 `from_addr`/`to_addr` 逐字一致。
2. **`root_cid` 的还原是否与一次性抽取一致**：对比同一 cid 在新老来源下的 `root_cid`（我的还原算法是
   mongo 侧 `IDCidMap`/`GetRootID` 的等价复刻，但没有真实响应可跑，只有单测）。
3. **分片边界重叠高度在回放时的行数**（见「已知差异 1」）。
4. **缺口回补范围**：`max(epoch)=6409152` → 开启时刻之间的高度必须由回填/replay 覆盖，本补丁不做。
5. 如果读取侧（另一条工作线）要复用 `po.LargeTransfer` / `repository.LargeTransferRepo`，请直接复用而不要
   另起一份同名类型，避免合并冲突。
