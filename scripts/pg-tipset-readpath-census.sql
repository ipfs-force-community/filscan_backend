-- =============================================================================
-- chain.sync_syncer_epochs 普查 / 「tipset 读路径能不能改读 PG」前置评估（只读）
-- =============================================================================
--
-- 跑法（在 backend 主机 172.31.34.109 上；本机已有一个从 /root/config.toml 抽 DSN 的包装）：
--
--     /root/ltr_pg.sh -f scripts/pg-tipset-readpath-census.sql
--     # 换同步器名 / 换评估区间：
--     /root/ltr_pg.sh -v NAME=sector -v FROM=6400000 -v TO=6412997 \
--         -f scripts/pg-tipset-readpath-census.sql
--
-- 本脚本**只读**：全文只有 SELECT 与 psql 的 \echo/\set（§0 有一条自证查询）。
-- 用 -f 跑（psql 元命令与 :变量 只在 -f 生效）；不要用 -c。
--
-- -----------------------------------------------------------------------------
-- 这张表是什么（写入路径，代码级依据）
-- -----------------------------------------------------------------------------
--   migration/11.syncer.sql:18   chain.sync_syncer_epochs(epoch, empty, keys varchar[],
--                                name, parent_keys varchar[], cost)，unique(epoch,name)
--   modules/syncer/syncer.go:891-917  isEmpty(current)：
--        parent = agg.ParentTipset(current)[0].Cids      （聚合器 /aggregators/parent_tipset）
--        empty  = len(agg.Tipset(current)) == 0          （聚合器 /aggregators/tipset 无该高度文档）
--   modules/syncer/syncer.go:930-950  prepareSyncerEpoch：
--        key = agg.Tipset(epoch)[0].Cids                 （聚合器 /aggregators/tipset）
--        empty=false 时 keys=该高度的 tipset cids；empty=true 时 keys=NULL
--   modules/syncer/syncer.go:1029     SaveSyncSyncerEpoch（每高度一行，ON CONFLICT DO NOTHING）
--
--   ⇒ keys / parent_keys 就是聚合器那两个端点的 Cids 数组的**逐字拷贝**，不是另一套口径。
--     所以 Cids 这一列是等价的；问题出在**别的列**（见下）。
--
-- -----------------------------------------------------------------------------
-- 结论：不能把它当成 Tipset / ParentTipset 的替代来源（2026-09-29 现场核实）
-- -----------------------------------------------------------------------------
-- 聚合器这两个端点返回的**结构**（不是只有 cids）：
--   londobell 侧无投影：pool-monitor/tipset.js      = [{$match:{_id: StartEpoch}}]
--                        pool-monitor/parent_tipset.js = [{$match:{_id:{$lt:EndEpoch}}},{$sort:{_id:-1}},{$limit:1}]
--                        （cmd/londobell-api/controller/aggregators/{tipset,parent_tipset}.go 把整个文档
--                          解进 model.TipSetRes，再被本仓 londobell.Tipset / londobell.ParentTipset 接住）
--   文档字段（bell 写入侧 racailum/segment/model/tipset.go:48-57）= 8 列：
--        _id(Epoch) / Cids / MinTimestamp / ChildEpoch / State / Receipts / Weight / BaseFee
--
-- 本仓这一侧真正被读的字段（grep 全部消费点）：
--   londobell.Tipset        → 只有 BaseFee（modules/filscan/acl/acl_block_chain.go:583-590，消息详情页）
--   londobell.ParentTipset  → ID、Cids、Weight、BaseFee、State
--        modules/filscan/assembler/assembler_block_chain_info.go:61-65
--        → filscan.BlockDetails{parents, parent_weight, parent_base_fee, state_root}
--        （api/api_block_chain.go:212-216，**区块详情页对外字段**）
--        + acl_block_chain.go:219-228（只读 ID 当 parentStart）
--
-- PG 侧能提供的列（全库 76 张表逐一核过，没有任何 tipset/block 类表，也没有
-- weight/state_root/receipts/base_fee/min_timestamp/child_epoch 这些列）：
--   ID   = epoch            ✓（本表）
--   Cids = keys             ✓（本表；NULL 时等价于聚合器返回空 —— 见 §4）
--   其余 6 列里：BaseFee 有**旁路来源**（见下），MinTimestamp/ChildEpoch/State/Receipts/Weight
--   这 5 列**全库无来源**（76 张表逐一核过：没有任何 tipset/block 类表，也没有
--   weight/state_root/receipts/min_timestamp/child_epoch 这些列）。
--       唯一沾边的 BaseFee 来源是 chain.base_gas_costs.base_gas（§6），由 trace-task 从
--        /adapter/epoch 的 tipset.BaseFee 写入（modules/syncer/chain/trace-task/trace_task.go:166-168）。
--
-- -----------------------------------------------------------------------------
-- 现场实测（2026-09-29 23:5x，backend-new01 → 聚合器 172.31.38.30:1237，逐值比对）
-- -----------------------------------------------------------------------------
--   ① keys == /aggregators/tipset 的 Cids            4/4 采样相等（含老高度 5,000,000）
--   ② parent_keys == /aggregators/parent_tipset Cids 6/6 相等（含两个 empty=true 高度）
--   ③ 行存在且 keys 为空 ⟺ 聚合器 /tipset 返回 data:null（23,828 = 23,828，双向 0 反例）
--   ④ 聚合器 /tipset 的 BaseFee == chain.base_gas_costs[E+1].base_gas：4/4 相等，且 4/4 ≠ base_gas[E]
--        例：E=6413031 AGG BaseFee=113159 == base_gas[6413032]=113159（base_gas[E]=100585）
--            E=6413030 → 100585 == [6413031]；E=6413000 → 102198 == [6413001]；E=5000000 → 100 == [5000001]
--        ⇒ bell 文档的 BaseFee 取的是 child 的 MinTicketBlock().ParentBaseFee（错一格）——已实测坐实，不是推断。
--   ⑤ 聚合器文档确实带真值：E=5000000 → MinTimestamp=1748306400、ChildEpoch=5000001、State=<bafy…>、
--      Receipts=<cid>、Weight=119327157957；近端高度 ChildEpoch=0 / Receipts=""（尚未回填）。
--   ⑥ 同一晚 23:52 前后该端口曾 connect refused（重启窗口），23:58 恢复 ⇒ 这条读路径的另一半风险
--      是「聚合器本身会抖」，不是数据一致性。
--
-- ⇒ 结论：**不要**做「PG 优先 + 回落」的 Tipset/ParentTipset 装饰器。
--    ParentTipset 若用 PG 拼，会把区块详情页的 parent_weight / state_root 变成 0/空（api.BlockDetails
--    的对外字段，用户可见的错值）；Tipset 虽然 API 侧只有 BaseFee 被读、且该值已实测可取，
--    但整结构仍有 5 列只能给零值 = 「不保证等价」，会给后来的消费点埋静默零值。
--    另：本表只覆盖 epoch >= 3,421,974（链前半段整段没有行），且区间内只有 3 个洞共 ~7.9 万高度
--    （§2/§3 会量出来）⇒ 即便只搬 Cids，也只能是「PG 优先 + 任一缺失/异常回落聚合器」，
--    且回落判据必须区分「行不存在（PG 缺口）」与「行存在但 keys 为空（链上真空高度，聚合器同样返回空）」。
--
-- -----------------------------------------------------------------------------
-- 可行的三条后续（都没做，按等价性×成本排序）
-- -----------------------------------------------------------------------------
--   A. 只搬 BaseFee（等价性已实测）：把「只读 BaseFee」的调用点改读 chain.base_gas_costs
--        · modules/filscan/acl/acl_block_chain.go:583-590  Tipset(message.Epoch-1) → base_gas[message.Epoch]
--        · modules/filscan/biz/browser/biz_output_imtoken.go:82 / :167  ParentTipset(x) → base_gas[doc_epoch+1]
--      代价：是标量读、天然可回落；但要处理「doc_epoch 未知」（x-1 为空时文档在更早高度，
--      由「< x 的最大非空 keys 高度」推出，并用「该区间 PG 无缺行」当可服务的判据）。
--   B. 只搬 Cids（对「只读 Cids 的消费者」等价）：现在只有同步器那两处（modules/syncer/syncer.go:891-950）
--      只吃 Cids，但那是写方读自己写的行（环形），叠加 97.4% 覆盖 ⇒ 收益/风险都不划算。
--   C. 真搬家：新建一张 8 列镜像表（epoch + cids[] + min_timestamp + child_epoch + state + receipts +
--      weight + base_fee，当前 PG 没有此表）。cids 可从本表回填、base_fee 可从 base_gas_costs 回填（A 的关系），
--      其余 4 列必须从聚合器/lotus 取 ⇒ 成本最大，但只有它能让 Tipset/ParentTipset 真正切读。
--
-- =============================================================================

-- §0 只读自证：把本会话设成默认只读 —— 之后本脚本里**任何**写语句都会直接报错，
--    所以「这份脚本没改生产」不是靠人看，而是靠引擎兜住。
\echo '== §0 只读自证：default_transaction_read_only =='
set default_transaction_read_only = on;
select current_setting('default_transaction_read_only') as read_only /* 期望 on */;

-- 评估参数（可用 -v 覆盖）
\if :{?NAME}
\else
  \set NAME chain
\endif
\if :{?FROM}
\else
  \set FROM 6400000
\endif
\if :{?TO}
\else
  \set TO 6412997
\endif

\echo ''
\echo '== §1 规模与边界（按 name；parent_keys 为空的计数如有非 0 即说明写路径异常） =='
select name,
       count(*)                                                              as rows_n,
       min(epoch)                                                            as min_epoch,
       max(epoch)                                                            as max_epoch,
       count(*) filter (where empty)                                         as empty_rows,
       count(*) filter (where keys is null or cardinality(keys) = 0)          as keys_null_or_empty,
       count(*) filter (where parent_keys is null or cardinality(parent_keys) = 0) as parent_keys_null
  from chain.sync_syncer_epochs
 group by name
 order by rows_n desc;

\echo ''
\echo '== §2 缺失高度数（唯一判据：missing=0 才谈「硬切」，否则只能回落式读路径） =='
with e as (
  select distinct epoch as ep from chain.sync_syncer_epochs where name = :'NAME'
)
select :'NAME'                                          as name,
       count(*)                                         as distinct_epochs,
       min(ep)                                          as min_ep,
       max(ep)                                          as max_ep,
       (max(ep) - min(ep) + 1)                          as span,
       (max(ep) - min(ep) + 1) - count(*)               as missing,
       round(100.0 * count(*) / (max(ep) - min(ep) + 1), 3) as cover_pct
  from e;

\echo ''
\echo '== §3 缺失高度分布：洞的个数 / 清单（起止 + 长度 + 换算天数）；d=1 的行是正常相邻 =='
with e as (
  select distinct epoch as ep from chain.sync_syncer_epochs where name = :'NAME'
), g as (
  select ep, ep - lag(ep) over (order by ep) as d from e
), h as (
  select ep - d + 1 as hole_start, ep - 1 as hole_end, d - 1 as hole_len from g where d > 1
)
select count(*)                        as holes,
       sum(hole_len)                    as missing_total,
       max(hole_len)                    as max_hole_len,
       count(*) filter (where hole_len >= 1000) as holes_ge1000
  from h;

with e as (
  select distinct epoch as ep from chain.sync_syncer_epochs where name = :'NAME'
), g as (
  select ep, ep - lag(ep) over (order by ep) as d from e
), h as (
  select ep - d + 1 as hole_start, ep - 1 as hole_end, d - 1 as hole_len from g where d > 1
)
select hole_start,
       hole_end,
       hole_len,
       round(hole_len * 30.0 / 86400.0, 2) as hole_days,
       to_char(to_timestamp(hole_start * 30 + 1598306400) at time zone 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI') as hole_start_cst,
       to_char(to_timestamp(hole_end   * 30 + 1598306400) at time zone 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI') as hole_end_cst
  from h
 order by hole_len desc;

\echo ''
\echo '== §4 语义校验：keys 为空 ⟺ empty（等价于聚合器 /tipset 在该高度返回空文档） =='
select count(*) filter (where empty and (keys is null or cardinality(keys) = 0))  as empty_and_keys_null,
       count(*) filter (where not empty and (keys is null or cardinality(keys) = 0)) as nonempty_but_keys_null,
       count(*) filter (where empty and cardinality(keys) > 0)                    as empty_but_keys_present
  from chain.sync_syncer_epochs
 where name = :'NAME';

\echo '-- tipset 文档 cids 个数分布（keys；对照 parent_keys 的个数） --'
select cardinality(keys) as cids_per_tipset, count(*) as n
  from chain.sync_syncer_epochs
 where name = :'NAME' and keys is not null and cardinality(keys) > 0
 group by 1 order by n desc limit 12;

\echo ''
\echo '== §5 给定区间的「回落率」估算（行不存在 ⇒ 必须回落聚合器；keys 为空 ⇒ 可等价返回空） =='
with r as (
  select epoch, empty, keys from chain.sync_syncer_epochs
   where name = :'NAME' and epoch between :FROM and :TO
)
select :FROM                                           as w_from,
       :TO                                             as w_to,
       (:TO - :FROM + 1)                               as window_heights,
       count(*)                                        as rows_present,
       (:TO - :FROM + 1) - count(*)                     as rows_missing_fallback,
       count(*) filter (where keys is not null and cardinality(keys) > 0) as servable_nonempty,
       count(*) filter (where empty)                    as provably_empty,
       round(100.0 * count(*) / (:TO - :FROM + 1), 2)    as row_present_pct
  from r;

\echo ''
\echo '== §6（备用窄路径）chain.base_gas_costs 覆盖：仅能补 BaseFee 一个标量，且与 tipset 文档错一格 =='
select count(*) as rows_n, min(epoch) as min_epoch, max(epoch) as max_epoch from chain.base_gas_costs;

\echo '-- 手工对照探针（聚合器可用时跑；拿 E 与 E+1 两行的 base_gas 去比聚合器 /aggregators/tipset 的 BaseFee，'
\echo '--   目的＝实测确认「文档 BaseFee 取的是 child 的 ParentBaseFee」这个错格关系，别按推断上线） --'
\echo '--   select epoch, base_gas from chain.base_gas_costs where epoch in (:FROM, :FROM + 1) order by epoch;'
\echo '--   curl -s -m 25 -X POST http://172.31.38.30:1237/aggregators/tipset -H "content-type: application/json" -d {"start"::FROM} '
