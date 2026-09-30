#!/usr/bin/env bash
#
# ops/wincount_gas_reward/rollout.sh
#
# wincount gas_reward 上线编排（**在 backend 主机 172.31.34.109 上以 root 跑**）。
#
# 严格按依赖顺序，每一步都有自己的判据；任何一步判据不过 = **停在这一步**（后续步骤不执行）。
#
#   ① migration  DDL：chain.miner_win_counts 加 gas_reward（不带 ONLY，级联到全部分区）+ 后检
#   ② binary     装新 filscan-syncer（备份 → stop → install → start → 三段判活）
#   ③ backfill   分批回填历史行（--max-partitions / --sleep），幂等、可中断
#   ④ config     最后才开 miner_wincount_read_from_pg 并重启 filscan-api（含前后金额比对）
#
# ===== 为什么顺序不能倒（硬约束）=====
#   新同步器写 4 列（epoch, miner, win_count, gas_reward）。**列不存在时 INSERT 直接失败**
#   ⇒ 写入链路（chain.miner_win_counts）整条挂掉，同步器每个高度都会写失败。
#   所以 ① 必须早于 ②。反过来（先装二进制后加列）会让同步器在「列还没加」的窗口里全部写失败。
#   另：本表带 INSERT 规则 range_insert_action_rule（→ action_miner_win_counts_range_insert(new.*)），
#   **不能用 ON CONFLICT**；加列必须**不带 ONLY**，否则分区少一列，规则里的 SELECT $1.* 直接报错。
#
# 用法（默认 dry-run，只打印不执行）：
#   rollout.sh --binary /root/wincount_gas_reward/filscan-syncer-wincount \
#              --expected-sha <sha256> [--step 2] [--max-partitions 4 --sleep 5] [--yes]
#
#   --step N      只跑第 N 步（1..4），便于逐步推进/复跑；缺省跑 1..4
#   --max-partitions / --sleep   第 3 步的分批参数（缺省 4 / 5s，避免一把梭）
#   --since H     第 3 步只回填 >= H 的分区
#   --yes         真的执行（缺省 dry-run）

set -euo pipefail

# ---------- 现场常量（2026-09-30 实测）----------
PSQL="${PSQL:-/root/ltr_pg.sh}"                 # 主机上现成的 psql 包装脚本
OPS_DIR="${OPS_DIR:-/root/wincount_gas_reward}" # 本目录在主机上的落地位置
MIGRATION_SQL="${MIGRATION_SQL:-$OPS_DIR/36.miner_win_counts_gas_reward.sql}"
PREFLIGHT_SQL="$OPS_DIR/01_preflight.sql"
BACKFILL_SH="$OPS_DIR/backfill.sh"
VALIDATE_SQL="$OPS_DIR/03_validate.sql"

SYNCER_SVC="filscan-syncer"
API_SVC="filscan-api"
SYNCER_BIN="/root/filscan-syncer"
SYNCER_LOG="/root/logs/filscan-syncer.log"
API_PORT=27000
CONFIG="/root/config.toml"
BAK_DIR="/root/deploy-bak"
FEATURE_KEY="miner_wincount_read_from_pg"
LEDGER_SQL="select max(epoch) from chain.miner_win_counts"   # 活体进度探针（实测 6413196→6413199 / 82s）

BINARY=""
EXPECT_SHA=""
ONLY_STEP=0
MAX_PARTITIONS=4
SLEEP=5
SINCE=""
CONFIRM=0

usage() { sed -n '2,32p' "${BASH_SOURCE[0]}"; exit 0; }
while [ $# -gt 0 ]; do
  case "$1" in
    --binary)         BINARY="$2"; shift 2 ;;
    --expected-sha)   EXPECT_SHA="$2"; shift 2 ;;
    --step)           ONLY_STEP="$2"; shift 2 ;;
    --max-partitions) MAX_PARTITIONS="$2"; shift 2 ;;
    --sleep)          SLEEP="$2"; shift 2 ;;
    --since)          SINCE="$2"; shift 2 ;;
    --yes|-y)         CONFIRM=1; shift ;;
    --help|-h)        usage ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

[ "$(id -u)" = "0" ] || { echo "必须在 backend 主机上以 root 跑（supervisorctl/psql 都需要）" >&2; exit 1; }
command -v supervisorctl >/dev/null || { echo "找不到 supervisorctl：确认这台就是 backend 主机" >&2; exit 1; }
[ -x "$PSQL" ] || { echo "找不到 $PSQL" >&2; exit 1; }

TS="$(date +%Y%m%d-%H%M%S)"
RUN_DIR="$BAK_DIR/wincount-rollout-$TS"
mkdir -p "$RUN_DIR"

log()  { printf '\n\033[1m[%s] %s\033[0m\n' "$(date +%H:%M:%S)" "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31m✗ %s\033[0m\n' "$*"; }
note() { printf '    %s\n' "$*"; }
run()  { if [ "$CONFIRM" -eq 1 ]; then "$@"; else printf '    (dry-run) %s\n' "$*"; fi; }
die()  { bad "$*"; echo; echo "== 停在当前步骤，后续步骤未执行 =="; exit 1; }

# psql_q <sql> → 无表头 | 分隔
psql_q() { "$PSQL" -At -F'|' -c "$1"; }

# ---------------------------------------------------------------- ① migration
step_migration() {
  log "① migration：给 chain.miner_win_counts 加 gas_reward（级联到全部分区）"

  local kind parts hascol
  IFS='|' read -r kind parts hascol <<<"$(psql_q "select c.relkind,
       (select count(*) from pg_inherits i where i.inhparent=c.oid),
       (select count(*) from pg_attribute a where a.attrelid=c.oid and a.attname='gas_reward' and a.attnum>0 and not a.attisdropped)
from pg_class c where c.oid='chain.miner_win_counts'::regclass;")"
  note "relkind=$kind partitions=$parts has_gas_reward_col=$hascol"
  [ "$kind" = "p" ] || die "chain.miner_win_counts 不是分区父表（relkind=$kind）—— 现场与设计不符"
  [ "${parts:-0}" -ge 300 ] || die "分区数 $parts 异常（期望 ≥300）"
  [ "$hascol" = "0" ] || { ok "gas_reward 列已存在（migration 已跑过，跳过 DDL）"; return 0; }

  # 加列前确认写入规则在位（决定「不能用 ON CONFLICT」「必须级联」两条约束）
  local rules
  rules="$(psql_q "select count(*) from pg_rewrite r join pg_class c on c.oid=r.ev_class
                   where c.relname in ('miner_win_counts','miner_rewards') and r.rulename='range_insert_action_rule';")"
  note "INSERT 规则条数=$rules（期望 2）"
  [ "${rules:-0}" -ge 2 ] || die "INSERT 规则不在位 —— 现场与 migration 的前提不符，停"

  run "$PSQL" -v ON_ERROR_STOP=1 -f "$MIGRATION_SQL"

  if [ "$CONFIRM" -eq 0 ]; then note "(dry-run) 后检略"; return 0; fi

  # 后检 1：所有分区都拿到 gas_reward（应为 0）
  local missing
  missing="$(psql_q "select count(*) from pg_inherits i join pg_class c on c.oid=i.inhrelid
                     where i.inhparent='chain.miner_win_counts'::regclass
                       and not exists (select 1 from pg_attribute a where a.attrelid=c.oid and a.attname='gas_reward' and a.attnum>0 and not a.attisdropped);")"
  note "分区缺 gas_reward 列的数量=$missing（期望 0）"
  [ "$missing" = "0" ] || die "有分区没拿到列 —— 规则里的 SELECT \$1.* 会列数不匹配、写入路径会挂。先修好再继续（回滚见 rollback.sh --step 3）"

  # 后检 2：列序号在父表与分区上必须一致（应为 0 行不一致）
  local mismatch
  mismatch="$(psql_q "with parent as (select a.attnum n from pg_attribute a where a.attrelid='chain.miner_win_counts'::regclass and a.attname='gas_reward')
select count(*) from pg_inherits i join pg_class c on c.oid=i.inhrelid
  join pg_attribute a on a.attrelid=c.oid and a.attname='gas_reward' and a.attnum>0
where i.inhparent='chain.miner_win_counts'::regclass and a.attnum <> (select n from parent);")"
  note "列序号不一致的分区数=$mismatch（期望 0）"
  [ "$mismatch" = "0" ] || die "列序号错位 ⇒ SELECT \$1.* 会静默错列。停，先修好"
  ok "migration 完成且两条后检通过"
}

# ---------------------------------------------------------------- ② binary
syncer_gates() {
  # 三段判活：RUNNING + /proc/<pid>/exe sha + 45s PID 稳定 + 日志无 panic/列不存在 + 台账推进
  local want_sha="$1"
  local st pid got
  st="$(supervisorctl status "$SYNCER_SVC" 2>/dev/null || true)"
  echo "$st" | grep -q RUNNING || return 1
  pid="$(echo "$st" | sed -n 's/.*pid \([0-9]*\).*/\1/p')"
  [ -n "$pid" ] || return 2
  got="$(sha256sum "/proc/$pid/exe" 2>/dev/null | awk '{print $1}')"
  [ "$got" = "$want_sha" ] || { bad "运行中二进制 sha=$got 期望=$want_sha"; return 3; }

  local before after
  before="$(psql_q "$LEDGER_SQL" || true)"
  sleep 45
  st="$(supervisorctl status "$SYNCER_SVC" 2>/dev/null || true)"
  echo "$st" | grep -q RUNNING || { bad "45s 后不在 RUNNING"; return 4; }
  [ "$(echo "$st" | sed -n 's/.*pid \([0-9]*\).*/\1/p')" = "$pid" ] || { bad "45s 内 PID 变了（$pid → 变了）"; return 5; }
  after="$(psql_q "$LEDGER_SQL" || true)"
  note "台账 max(epoch)：$before → $after"
  [ -n "$after" ] && [ -n "$before" ] && [ "$after" -gt "$before" ] 2>/dev/null || { bad "任务台账未推进"; return 6; }

  # 新追加的日志里不能有 panic / 列不存在
  local tail_txt
  tail_txt="$(tail -c 400000 "$SYNCER_LOG" 2>/dev/null || true)"
  if echo "$tail_txt" | grep -qE 'panic:|column "gas_reward"|gas_reward.*does not exist|INSERT has more expressions'; then
    bad "日志出现 panic / 列不匹配"; echo "$tail_txt" | grep -nE 'panic:|gas_reward' | tail -5; return 7
  fi
  # 建议项（非硬判据）：新高度应当开始带 gas_reward
  local fresh
  fresh="$(psql_q "select count(*) from chain.miner_win_counts where epoch >= $after and gas_reward is not null;" || echo '?')"
  note "新高度（epoch>=$after）已带 gas_reward 的行数=$fresh（>0 表示写入路径确实在写新列）"
  return 0
}

step_binary() {
  log "② binary：装新 filscan-syncer（备份 → stop → install → start → 判活）"
  [ -n "$BINARY" ] || die "必须给 --binary <新二进制路径>"
  [ -f "$BINARY" ] || die "找不到 $BINARY"
  local new_sha
  new_sha="$(sha256sum "$BINARY" | awk '{print $1}')"
  note "新二进制：$BINARY sha=$new_sha size=$(stat -c %s "$BINARY")"
  if [ -n "$EXPECT_SHA" ] && [ "$new_sha" != "$EXPECT_SHA" ]; then
    die "新二进制 sha 与 --expected-sha 不符（$new_sha != $EXPECT_SHA）—— 指纹门不过，拒绝安装"
  fi
  if [ "$CONFIRM" -eq 1 ] && [ "$new_sha" = "$(sha256sum "$SYNCER_BIN" | awk '{print $1}')" ]; then
    ok "现役二进制已是这份（sha 相同），跳过换件"; return 0
  fi

  local bak="$BAK_DIR/filscan-syncer.bak-$TS"
  local old_sha; old_sha="$(sha256sum "$SYNCER_BIN" | awk '{print $1}')"
  note "现役 sha=$old_sha → 备份到 $bak"

  if [ "$CONFIRM" -eq 0 ]; then
    note "(dry-run) cp -a $SYNCER_BIN $bak"
    note "(dry-run) supervisorctl stop $SYNCER_SVC"
    note "(dry-run) install -m 755 $BINARY $SYNCER_BIN"
    note "(dry-run) supervisorctl start $SYNCER_SVC"
    note "(dry-run) 三段判活：RUNNING + /proc/<pid>/exe sha + 45s PID 稳定 + 日志无 panic + 台账 max(epoch) 推进"
    return 0
  fi

  cp -a "$SYNCER_BIN" "$bak" || die "备份失败 —— 未做任何改动"
  echo "$old_sha" > "$RUN_DIR/syncer.old.sha"

  supervisorctl stop "$SYNCER_SVC" >/dev/null || die "stop 失败（未改动二进制，直接重跑即可）"
  install -m 755 "$BINARY" "$SYNCER_BIN" || { bad "install 失败，回滚二进制"; cp -a "$bak" "$SYNCER_BIN"; supervisorctl start "$SYNCER_SVC" >/dev/null; die "install 失败"; }
  [ "$(sha256sum "$SYNCER_BIN" | awk '{print $1}')" = "$new_sha" ] || die "落地后 sha 不符 —— 回滚：cp -a $bak $SYNCER_BIN && supervisorctl start $SYNCER_SVC"

  supervisorctl start "$SYNCER_SVC" >/dev/null || true

  if syncer_gates "$new_sha"; then
    ok "新同步器上线并判活通过（含 45s PID 稳定与台账推进）"
    echo "$new_sha" > "$RUN_DIR/syncer.new.sha"
    return 0
  fi

  bad "判活不过 —— 自动恢复备份并重启"
  supervisorctl stop "$SYNCER_SVC" >/dev/null || true
  cp -a "$bak" "$SYNCER_BIN"
  supervisorctl start "$SYNCER_SVC" >/dev/null || true
  sleep 10
  supervisorctl status "$SYNCER_SVC" | grep -q RUNNING \
    && ok "已恢复到旧二进制（sha=$old_sha）并在 RUNNING" \
    || bad "恢复后仍不在 RUNNING —— 立即人工介入（备份：$bak）"
  die "② binary 判活失败，已回滚到备份；**③④ 未执行**"
}

# ---------------------------------------------------------------- ③ backfill
step_backfill() {
  log "③ backfill：分批回填历史行（幂等；只补 gas_reward is null）"
  [ -x "$BACKFILL_SH" ] || die "找不到 $BACKFILL_SH"
  [ "$CONFIRM" -eq 1 ] || { note "(dry-run) export PSQL_CMD=$PSQL; $BACKFILL_SH --dry-run --max-partitions $MAX_PARTITIONS"; return 0; }

  local hascol
  hascol="$(psql_q "select count(*) from pg_attribute a where a.attrelid='chain.miner_win_counts'::regclass and a.attname='gas_reward' and a.attnum>0 and not a.attisdropped;")"
  [ "$hascol" = "1" ] || die "gas_reward 列不存在 —— ③ 必须晚于 ①（先跑 --step 1）"

  export PSQL_CMD="$PSQL"
  local extra=""
  [ -n "$SINCE" ] && extra="--since $SINCE"
  note "先 dry-run 看影响面"
  "$BACKFILL_SH" --dry-run --max-partitions "$MAX_PARTITIONS" $extra || die "dry-run 失败"

  note "正式回填（分批：每批 $MAX_PARTITIONS 个分区、批内 sleep ${SLEEP}s）"
  "$BACKFILL_SH" --yes --max-partitions "$MAX_PARTITIONS" --sleep "$SLEEP" $extra || die "回填批次失败（可中断/可续跑：重跑本步即可）"

  log "③ 校验：negative_rows 必须 0"
  "$PSQL" -At -F'|' -f "$VALIDATE_SQL" > "$RUN_DIR/validate.$TS.txt" 2>&1 || die "校验脚本失败（见 $RUN_DIR/validate.$TS.txt）"
  local negrows
  negrows="$(awk -F'|' 'NF==5 && $5+0>0 {n++} END{print n+0}' "$RUN_DIR/validate.$TS.txt")"
  note "negative_rows>0 的分区数=$negrows（期望 0；非 0 = penalty>0 或奖励 actor 余额不足，需人工核对聚合器）"
  [ "$negrows" = "0" ] || die "出现负值分区 —— 停，不要开 ④；用 filscan-agg-parity 核对那批高度"
  ok "回填完成且校验通过（明细：$RUN_DIR/validate.$TS.txt）"
}

# ---------------------------------------------------------------- ④ config
api_smoke() {
  # 返回 0 = api 活着；顺带打印 FinalHeight
  local out
  out="$(curl -s -m 20 -X POST "http://127.0.0.1:$API_PORT/api/v1/FinalHeight" -H 'Content-Type: application/json' -d '{}' || true)"
  echo "$out" | grep -q '"height"' || return 1
  note "FinalHeight: $out"
  return 0
}

api_block_snapshot() {
  # 取一个真实区块的 tx_fee_reward / mined_reward，用于开关前后比对
  local height="$1" cid bd
  cid="$(curl -s -m 25 -X POST "http://127.0.0.1:$API_PORT/api/v1/TipsetDetail" -H 'Content-Type: application/json' \
         -d "{\"height\":$height}" | sed -n 's/.*"cid":"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$cid" ] || { echo "NO_CID"; return 1; }
  bd="$(curl -s -m 30 -X POST "http://127.0.0.1:$API_PORT/api/v1/BlockDetails" -H 'Content-Type: application/json' \
        -d "{\"block_cid\":\"$cid\"}")"
  echo "$bd" | python3 -c '
import sys, json
d = json.load(sys.stdin)["result"]["block_details"]["block_basic"]
print(json.dumps({k: d.get(k) for k in ("cid","tx_fee_reward","mined_reward","reward")}, ensure_ascii=False))
' 2>/dev/null || echo "PARSE_FAIL"
}

step_config() {
  log "④ config：最后才开 $FEATURE_KEY（改 /root/config.toml + 重启 $API_SVC）"
  local probe_height="${PROBE_HEIGHT:-6413038}"
  note "开关前基线（高度 $probe_height）"
  local before=""
  [ "$CONFIRM" -eq 1 ] && { before="$(api_block_snapshot "$probe_height")"; note "before: $before"; }

  if [ "$CONFIRM" -eq 0 ]; then
    note "(dry-run) 备份 $CONFIG → $BAK_DIR/config.toml.bak-$TS"
    note "(dry-run) 在 [feature] 写入 $FEATURE_KEY = true"
    note "(dry-run) supervisorctl restart $API_SVC"
    note "(dry-run) 判据：RUNNING + FinalHeight 200 + 同区块 tx_fee_reward/mined_reward 与 before 逐值一致"
    return 0
  fi

  cp -a "$CONFIG" "$BAK_DIR/config.toml.bak-$TS" || die "配置备份失败，未改配置"
  python3 - "$CONFIG" "$FEATURE_KEY" <<'PY' || die "配置写入失败（已备份，可 cp -a 回去）"
import re, sys
path, key = sys.argv[1], sys.argv[2]
lines = open(path, encoding="utf-8").read().splitlines()
pat = re.compile(r'^\s*' + re.escape(key) + r'\s*=')
out, in_feature, done = [], False, False
for ln in lines:
    if ln.strip().startswith("[") :
        if in_feature and not done:
            out.append(f"{key} = true"); done = True
        in_feature = ln.strip() == "[feature]"
    if pat.match(ln):
        out.append(f"{key} = true"); done = True; continue
    out.append(ln)
if in_feature and not done:
    out.append(f"{key} = true"); done = True
if not done:
    sys.exit("没有找到 [feature] 段")
open(path, "w", encoding="utf-8").write("\n".join(out) + "\n")
PY
  grep -qE "^$FEATURE_KEY\s*=\s*true" "$CONFIG" || die "写入后校验失败"
  ok "配置已写入（已备份 $BAK_DIR/config.toml.bak-$TS）"

  supervisorctl restart "$API_SVC" >/dev/null || true
  sleep 12
  supervisorctl status "$API_SVC" | grep -q RUNNING || {
    bad "api 不在 RUNNING —— 回滚配置"
    cp -a "$BAK_DIR/config.toml.bak-$TS" "$CONFIG"; supervisorctl restart "$API_SVC" >/dev/null || true
    die "④ 失败，配置已回滚（开关关闭 = 回到今天的行为）"
  }
  api_smoke || { bad "FinalHeight 不通 —— 回滚配置"; cp -a "$BAK_DIR/config.toml.bak-$TS" "$CONFIG"; supervisorctl restart "$API_SVC" >/dev/null || true; die "④ 失败，配置已回滚"; }

  local after; after="$(api_block_snapshot "$probe_height")"
  note "after : $after"
  if [ "$before" != "$after" ]; then
    bad "开关前后金额不一致！before=$before after=$after"
    note "回滚配置（这是「读 PG 与聚合器口径不一致」的红灯，必须查清再上）"
    cp -a "$BAK_DIR/config.toml.bak-$TS" "$CONFIG"; supervisorctl restart "$API_SVC" >/dev/null || true
    die "④ 失败：口径不一致，已回滚配置"
  fi
  ok "④ 完成：开关已开，金额与开关前逐值一致"
}

# ---------------------------------------------------------------- 主流程
echo "=== wincount gas_reward 上线编排 ==="
echo "模式      = $([ "$CONFIRM" -eq 1 ] && echo '正式执行' || echo 'dry-run（只打印，不执行）')"
if [ "$ONLY_STEP" -eq 0 ]; then STEP_LABEL="1→2→3→4"; else STEP_LABEL="只跑第 $ONLY_STEP 步"; fi
echo "步骤      = $STEP_LABEL"
echo "backfill  = max_partitions=$MAX_PARTITIONS sleep=${SLEEP}s since=${SINCE:-<全部>}"
echo "run 目录  = $RUN_DIR"
[ "$CONFIRM" -eq 1 ] || echo "!! 这是 dry-run：不改 config、不跑 DDL、不装二进制、不回填"

if [ -f "$PREFLIGHT_SQL" ]; then
  log "⓪ 只读体检（01_preflight.sql）"
  "$PSQL" -At -F'|' -f "$PREFLIGHT_SQL" | tee "$RUN_DIR/preflight.$TS.txt" | sed 's/^/    /'
else
  note "（没有 $PREFLIGHT_SQL，跳过体检）"
fi

case "$ONLY_STEP" in
  0) ;;
  1) step_migration ;;
  2) step_binary ;;
  3) step_backfill ;;
  4) step_config ;;
  *) step_migration; step_binary; step_backfill; step_config ;;
esac

log "收口"
echo "  备份/产物：$RUN_DIR"
echo "  下一步（开关打开后）："
echo "    ./bin/agg-parity -c $CONFIG -endpoints wincount -start <s> -end <e>   # 口径通关：差异 0、Unresolved 为空"
echo "    /block/<height> 的 TxFeeReward / MinedReward 与开关前一致（本脚本 ④ 已自动比对一个高度）"
echo "  巡检项（回填不持久）：03_validate.sql 的 NULL 行数，非 0 就重跑 backfill.sh（幂等）"
