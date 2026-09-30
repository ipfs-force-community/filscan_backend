#!/usr/bin/env bash
#
# ops/wincount_gas_reward/rollback.sh
#
# wincount gas_reward 回滚（**在 backend 主机 172.31.34.109 上以 root 跑**）。
#
# 严格按**逆序**回滚，每一步都可单独停：
#   ① 先关读开关（miner_wincount_read_from_pg=false）并重启 filscan-api
#      —— 读路径回到聚合器；此时 PG 侧有没有 gas_reward 都无所谓（未回填区间本来也会回落）。
#   ② 再把 filscan-syncer 回滚到**不写 gas_reward 的旧版本**并重启
#      —— 关键：只要写路径还在写 gas_reward，就**不能删列**（下一次同步的 INSERT 会因列不存在而失败）。
#   ③ 最后才动 DDL（默认只**改名**保留数据；`--drop-column` 才真删）
#
# ===== 中间态（列已加、开关关着）是完全安全的 =====
#   * 读路径回落聚合器 ⇒ /block/<height> 的 TxFeeReward / MinedReward 与改造前逐值一致；
#   * 新同步器继续写 gas_reward ⇒ 新数据在攒，将来重开开关不用重来；
#   * 列是**可空**的（无 default）⇒ 不写它的旧代码/旧 SQL 完全不受影响（不是 NOT NULL、无约束）。
#   ⇒ 所以最推荐的「回滚」其实是**只做第 ① 步**：零风险、可秒级回退、数据不丢。
#   ⇒ 第 ③ 步只有在「确定不再做这件事」时才需要。
#
# 用法（默认 dry-run）：
#   rollback.sh --yes                    # 逆序全走（③ 只改名）
#   rollback.sh --step 1 --yes           # 只关开关（推荐的最小回滚）
#   rollback.sh --step 3 --drop-column --yes   # 真删列（会丢回填结果，需 ② 已完成）
#   rollback.sh --binary /root/deploy-bak/filscan-syncer.bak-20260929-195644 --step 2 --yes

set -euo pipefail

PSQL="${PSQL:-/root/ltr_pg.sh}"
OPS_DIR="${OPS_DIR:-/root/wincount_gas_reward}"
CONFIG="/root/config.toml"
BAK_DIR="/root/deploy-bak"
API_SVC="filscan-api"
SYNCER_SVC="filscan-syncer"
SYNCER_BIN="/root/filscan-syncer"
SYNCER_LOG="/root/logs/filscan-syncer.log"
API_PORT=27000
FEATURE_KEY="miner_wincount_read_from_pg"
LEDGER_SQL="select max(epoch) from chain.miner_win_counts"

ONLY_STEP=0
ROLLBACK_BINARY=""
DROP_COLUMN=0
CONFIRM=0

usage() { sed -n '2,30p' "${BASH_SOURCE[0]}"; exit 0; }
while [ $# -gt 0 ]; do
  case "$1" in
    --binary)      ROLLBACK_BINARY="$2"; shift 2 ;;
    --step)        ONLY_STEP="$2"; shift 2 ;;
    --drop-column) DROP_COLUMN=1; shift ;;
    --yes|-y)      CONFIRM=1; shift ;;
    --help|-h)     usage ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

[ "$(id -u)" = "0" ] || { echo "必须在 backend 主机上以 root 跑" >&2; exit 1; }
[ -x "$PSQL" ] || { echo "找不到 $PSQL" >&2; exit 1; }
TS="$(date +%Y%m%d-%H%M%S)"
RUN_DIR="$BAK_DIR/wincount-rollback-$TS"; mkdir -p "$RUN_DIR"

log()  { printf '\n\033[1m[%s] %s\033[0m\n' "$(date +%H:%M:%S)" "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31m✗ %s\033[0m\n' "$*"; }
note() { printf '    %s\n' "$*"; }
run()  { if [ "$CONFIRM" -eq 1 ]; then "$@"; else printf '    (dry-run) %s\n' "$*"; fi; }
die()  { bad "$*"; echo; echo "== 停在当前步骤，后续步骤未执行 =="; exit 1; }
psql_q() { "$PSQL" -At -F'|' -c "$1"; }

# ---------------------------------------------------------------- ① 关开关
step_config_off() {
  log "① 关读开关 $FEATURE_KEY（重启 $API_SVC）"
  local bak="$BAK_DIR/config.toml.bak-$TS"
  local before=""
  if [ "$CONFIRM" -eq 1 ]; then
    before="$(curl -s -m 30 -X POST "http://127.0.0.1:$API_PORT/api/v1/FinalHeight" -H 'Content-Type: application/json' -d '{}' || true)"
    note "api 现状: ${before:-<不通>}"
    if ! grep -qE "^$FEATURE_KEY\s*=\s*true" "$CONFIG"; then
      ok "开关本来就是关的（未开启），① 无需动作"
      return 0
    fi
  fi

  if [ "$CONFIRM" -eq 0 ]; then
    note "(dry-run) cp -a $CONFIG $bak"
    note "(dry-run) 把 $FEATURE_KEY 改成 false（或删掉该行 = 默认 false）"
    note "(dry-run) supervisorctl restart $API_SVC"
    note "(dry-run) 判据：RUNNING + FinalHeight 200（读路径回到聚合器）"
    return 0
  fi

  cp -a "$CONFIG" "$bak" || die "配置备份失败，未改配置"
  python3 - "$CONFIG" "$FEATURE_KEY" <<'PY' || die "配置改写失败（备份在 $bak）"
import re, sys
path, key = sys.argv[1], sys.argv[2]
out = []
for ln in open(path, encoding="utf-8").read().splitlines():
    out.append(f"{key} = false" if re.match(r'^\s*' + re.escape(key) + r'\s*=', ln) else ln)
open(path, "w", encoding="utf-8").write("\n".join(out) + "\n")
PY
  supervisorctl restart "$API_SVC" >/dev/null || true
  sleep 12
  supervisorctl status "$API_SVC" | grep -q RUNNING || {
    bad "api 不在 RUNNING —— 恢复配置备份"
    cp -a "$bak" "$CONFIG"; supervisorctl restart "$API_SVC" >/dev/null || true
    die "① 失败并已恢复原配置"
  }
  local out
  out="$(curl -s -m 20 -X POST "http://127.0.0.1:$API_PORT/api/v1/FinalHeight" -H 'Content-Type: application/json' -d '{}' || true)"
  echo "$out" | grep -q '"height"' || { bad "FinalHeight 不通 —— 恢复配置备份"; cp -a "$bak" "$CONFIG"; supervisorctl restart "$API_SVC" >/dev/null || true; die "① 失败并已恢复原配置"; }
  ok "① 完成：开关已关、api 活着（读路径回到聚合器；PG 侧是否有 gas_reward 都无所谓）"
  note "FinalHeight: $out"
}

# ---------------------------------------------------------------- ② 回滚二进制
step_binary() {
  log "② 回滚 $SYNCER_SVC 二进制（不写 gas_reward 的旧版本）"
  local target="$ROLLBACK_BINARY"
  if [ -z "$target" ]; then
    target="$(ls -1t "$BAK_DIR"/filscan-syncer.bak-* 2>/dev/null | head -1 || true)"
  fi
  [ -n "$target" ] && [ -f "$target" ] || die "找不到可回滚的备份（用 --binary 指定）"
  local tsha; tsha="$(sha256sum "$target" | awk '{print $1}')"
  note "回滚目标：$target sha=$tsha size=$(stat -c %s "$target")"

  local cur_sha; cur_sha="$(sha256sum "$SYNCER_BIN" | awk '{print $1}')"
  if [ "$CONFIRM" -eq 1 ] && [ "$cur_sha" = "$tsha" ]; then ok "现役已经是这份，无需换件"; return 0; fi

  local pre_bak="$BAK_DIR/filscan-syncer.bak-$TS"
  if [ "$CONFIRM" -eq 0 ]; then
    note "(dry-run) cp -a $SYNCER_BIN $pre_bak      # 先留一份现役，防止回滚目标有问题"
    note "(dry-run) supervisorctl stop $SYNCER_SVC"
    note "(dry-run) install -m 755 $target $SYNCER_BIN"
    note "(dry-run) supervisorctl start $SYNCER_SVC"
    note "(dry-run) 判据：RUNNING + /proc/<pid>/exe sha == $tsha + 45s PID 稳定 + 日志无 panic + 台账推进"
    return 0
  fi

  cp -a "$SYNCER_BIN" "$pre_bak" || die "现役备份失败，未改动"
  supervisorctl stop "$SYNCER_SVC" >/dev/null || die "stop 失败"
  install -m 755 "$target" "$SYNCER_BIN" || { cp -a "$pre_bak" "$SYNCER_BIN"; supervisorctl start "$SYNCER_SVC" >/dev/null; die "install 失败，已还原现役"; }
  supervisorctl start "$SYNCER_SVC" >/dev/null || true

  sleep 5
  local st pid got before after
  st="$(supervisorctl status "$SYNCER_SVC" 2>/dev/null || true)"
  pid="$(echo "$st" | sed -n 's/.*pid \([0-9]*\).*/\1/p')"
  got="$(sha256sum "/proc/$pid/exe" 2>/dev/null | awk '{print $1}')"
  before="$(psql_q "$LEDGER_SQL" || true)"; sleep 45; after="$(psql_q "$LEDGER_SQL" || true)"
  note "台账 max(epoch)：$before → $after"

  local fail=0
  echo "$st" | grep -q RUNNING || fail=1
  [ "$got" = "$tsha" ] || { bad "/proc/pid/exe sha=$got 期望=$tsha"; fail=1; }
  [ -n "$after" ] && [ -n "$before" ] && [ "$after" -gt "$before" ] 2>/dev/null || { bad "台账未推进"; fail=1; }
  tail -c 300000 "$SYNCER_LOG" 2>/dev/null | grep -qE 'panic:' && { bad "日志有 panic"; fail=1; }

  if [ "$fail" -eq 0 ]; then
    ok "② 完成：旧版本在跑、判活通过（写路径不再写 gas_reward）"
    return 0
  fi

  bad "回滚判活不过 —— 把现役（$pre_bak）装回去"
  supervisorctl stop "$SYNCER_SVC" >/dev/null || true
  cp -a "$pre_bak" "$SYNCER_BIN"
  supervisorctl start "$SYNCER_SVC" >/dev/null || true
  sleep 10
  supervisorctl status "$SYNCER_SVC" | grep -q RUNNING && ok "已还原到回滚前状态" || bad "还原后仍异常 —— 人工介入（$pre_bak）"
  die "② 失败，**③ 未执行**（列保持原样，绝不在写路径未回滚前动 DDL）"
}

# ---------------------------------------------------------------- ③ DDL
step_ddl() {
  log "③ DDL：处理 gas_reward 列"
  local hascol
  hascol="$(psql_q "select count(*) from pg_attribute a where a.attrelid='chain.miner_win_counts'::regclass and a.attname='gas_reward' and a.attnum>0 and not a.attisdropped;")"
  [ "$hascol" = "1" ] || { ok "gas_reward 列已不存在，③ 无需动作"; return 0; }

  if [ "$DROP_COLUMN" -eq 1 ]; then
    # 硬前提：没有任何进程在写 gas_reward ⇒ 必须先完成 ②
    local svc_sha; svc_sha="$(sha256sum "$SYNCER_BIN" | awk '{print $1}')"
    local st pid running_sha
    st="$(supervisorctl status "$SYNCER_SVC" 2>/dev/null || true)"
    pid="$(echo "$st" | sed -n 's/.*pid \([0-9]*\).*/\1/p')"
    running_sha="$(sha256sum "/proc/$pid/exe" 2>/dev/null | awk '{print $1}' || true)"
    note "现役 $SYNCER_SVC sha=$running_sha"
    if [ "$running_sha" != "$svc_sha" ]; then
      note "（提示）进程与磁盘上的 $SYNCER_BIN sha 不一致，以进程为准判断；确认它是不写 gas_reward 的版本再继续"
    fi
    if [ "$CONFIRM" -eq 1 ]; then
      read -r -p "  确认 ② 已完成（写路径不再写 gas_reward）？输入 DROP 继续：" a || a=""
      [ "$a" = "DROP" ] || die "未确认，放弃删列（列保持原样，安全）"
    fi
    note "执行：alter table chain.miner_win_counts drop column if exists gas_reward;  （不带 ONLY，级联全部分区）"
    run "$PSQL" -v ON_ERROR_STOP=1 -c "alter table chain.miner_win_counts drop column if exists gas_reward;"
    if [ "$CONFIRM" -eq 1 ]; then
      local left
      left="$(psql_q "select count(*) from pg_inherits i join pg_class c on c.oid=i.inhrelid join pg_attribute a on a.attrelid=c.oid and a.attname='gas_reward' and a.attnum>0 where i.inhparent='chain.miner_win_counts'::regclass;")"
      note "仍带该列的分区数=$left（期望 0）"
      [ "$left" = "0" ] || die "有分区没删掉列 —— 分区与父表结构不一致，立刻人工修"
    fi
    bad "注意：删列会丢掉全部回填结果，重来需要重跑 backfill.sh"
    ok "③ 完成（drop column）"
    return 0
  fi

  note "安全做法（默认）：只改名保留数据"
  note "执行：alter table chain.miner_win_counts rename column gas_reward to gas_reward_disabled;  （不带 ONLY）"
  run "$PSQL" -v ON_ERROR_STOP=1 -c "alter table chain.miner_win_counts rename column gas_reward to gas_reward_disabled;"
  ok "③ 完成（rename，数据留档；改名可逆：rename column gas_reward_disabled to gas_reward）"
  note "若确认不再需要，再跑一次：rollback.sh --step 3 --drop-column --yes"
}

# ---------------------------------------------------------------- 主流程
echo "=== wincount gas_reward 回滚（逆序）==="
echo "模式   = $([ "$CONFIRM" -eq 1 ] && echo '正式执行' || echo 'dry-run（只打印）')"
if [ "$ONLY_STEP" -eq 0 ]; then STEP_LABEL="1→2→3（逆序）"; else STEP_LABEL="只跑第 $ONLY_STEP 步"; fi
echo "步骤   = $STEP_LABEL"
echo "DDL 动作 = $([ "$DROP_COLUMN" -eq 1 ] && echo 'drop column（丢回填数据）' || echo 'rename（保留数据，推荐）')"
echo "产物   = $RUN_DIR"
echo
echo "!! 顺序铁律：① 开关 → ② 二进制 → ③ DDL。"
echo "!! 只要写路径还在写 gas_reward，就不能删列（下一次同步 INSERT 会因列不存在而失败）。"
echo "!! 最推荐的「回滚」= 只做 ①（列留着、开关关着 = 完全安全的中间态）。"

case "$ONLY_STEP" in
  0) step_config_off; step_binary; step_ddl ;;
  1) step_config_off ;;
  2) step_binary ;;
  3) step_ddl ;;
  *) step_config_off; step_binary; step_ddl ;;
esac

log "收口"
echo "  回滚后必做："
echo "    * filscan-api：/block/<height> 的 TxFeeReward / MinedReward 正常（读路径已回聚合器）"
echo "    * filscan-syncer：日志里 reward-task 正常落库、无「列不存在」报错"
echo "    * 若做了 --drop-column：把 03_validate.sql 从巡检项移除（否则一直报列不存在）"
echo "  产物/备份：$RUN_DIR"
