#!/usr/bin/env bash
#
# ops/wincount_gas_reward/backfill.sh —— 按周分区回填 chain.miner_win_counts.gas_reward
#
# 只读 + 单分区 UPDATE（见 02_backfill.sql 的口径说明）。**不碰 mongo / 聚合器**。
#
# 用法（线上：PG 用主机上现成的包装脚本）
#   export PSQL_CMD=/root/ltr_pg.sh
#   ops/wincount_gas_reward/backfill.sh --dry-run                 # 先看要改多少行（不发写）
#   ops/wincount_gas_reward/backfill.sh --since 6300000 --yes     # 从某高度起正式回填
#   ops/wincount_gas_reward/backfill.sh --max-partitions 4 --sleep 5 --yes
#   ops/wincount_gas_reward/backfill.sh --only miner_win_counts_w40_2026_09_28_6407280_6427440 --yes
#
# PSQL_CMD 的约定：脚本会自己追加 `-At -F'|' -c "<sql>"`。
#   线上：PSQL_CMD=/root/ltr_pg.sh
#   本地：PSQL_CMD="psql -h 127.0.0.1 -p 5433 -U postgres -d filscan_probe"
#
# 幂等：只更新 `gas_reward is null` 的行 ⇒ 重复跑不会重复写、也不会覆盖已回填的值。
# 可中断：每个分区一个独立事务，Ctrl-C / 断连只丢当前分区，重跑接着来。
# 安全：默认 dry-run；不带 --yes 不会执行任何写。

set -euo pipefail

PSQL_CMD="${PSQL_CMD:-psql}"
SQL_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/02_backfill.sql"

SINCE=""
UNTIL=""
ONLY=""
MAX_PARTITIONS=0
SLEEP=0
# MODE 必须显式选择：空 = 拒绝执行（不默认 dry-run 也不默认写）
MODE=""

usage() { sed -n '2,20p' "${BASH_SOURCE[0]}"; exit 0; }

while [ $# -gt 0 ]; do
  case "$1" in
    --since) SINCE="$2"; shift 2 ;;
    --until) UNTIL="$2"; shift 2 ;;
    --only)  ONLY="$2"; shift 2 ;;
    --max-partitions) MAX_PARTITIONS="$2"; shift 2 ;;
    --sleep) SLEEP="$2"; shift 2 ;;
    --dry-run) MODE="dry"; shift ;;
    --yes|-y) MODE="write"; shift ;;
    --help|-h) usage ;;
    *) echo "未知参数: $1（--help 看用法）" >&2; exit 2 ;;
  esac
done

[ -f "$SQL_FILE" ] || { echo "找不到 $SQL_FILE" >&2; exit 1; }
case "$MODE" in
  dry)   DRY_RUN=1 ;;
  write) DRY_RUN=0 ;;
  *)
    echo "必须显式选择模式：--dry-run（只统计影响面，不写）或 --yes（正式回填）。" >&2
    echo "先跑 --dry-run 看影响面，再决定是否 --yes。" >&2
    exit 2
    ;;
esac

# run_sql <sql>：统一走 PSQL_CMD，无表头、| 分隔
run_sql() { $PSQL_CMD -At -F'|' -c "$1"; }

# 待回填分区的 (名称|下界|上界)，按高度升序。
# 边界从 relpartbound 里取，不靠分区命名规则猜。
list_partitions_sql=$(cat <<'SQL'
select c.relname
       ||'|'|| (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'FROM ..([0-9]+).'))[1]
       ||'|'|| (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO ..([0-9]+).'))[1]
from pg_inherits i
         join pg_class c on c.oid = i.inhrelid
where i.inhparent = 'chain.miner_win_counts'::regclass
order by (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'FROM ..([0-9]+).'))[1]::bigint
SQL
)

# 生成某个分区的「待更新行数」SQL（与 02_backfill.sql 的 WHERE 同口径，只把 update 换成 count）
count_sql_for() {
  local lo="$1" hi="$2"
  cat <<SQL
with ter as (select epoch from chain.builtin_actor_states
             where actor = 'f02' and epoch >= $lo and epoch < $hi
               and state ->> 'ThisEpochReward' is not null),
     r as (select distinct on (epoch, miner) epoch, miner
           from chain.miner_rewards
           where epoch >= $lo and epoch < $hi
           order by epoch, miner, block_time desc nulls last)
select count(*)
from chain.miner_win_counts w
         join r on r.epoch = w.epoch and r.miner = w.miner
         join ter on ter.epoch = r.epoch
where w.gas_reward is null
  and w.win_count is not null
  and w.epoch >= $lo
  and w.epoch < $hi
SQL
}

# 把 02_backfill.sql 的 :lo/:hi 占位符替换成具体边界（SQL 注释里的提及一并替换，无副作用）
update_sql_for() {
  local lo="$1" hi="$2"
  sed -e "s/:lo/$lo/g" -e "s/:hi/$hi/g" "$SQL_FILE"
}

echo "PSQL_CMD = $PSQL_CMD"
echo "SQL_FILE = $SQL_FILE"
echo "模式     = $([ "$DRY_RUN" -eq 1 ] && echo 'dry-run（不发写）' || echo '正式回填')"
echo "范围     = since=${SINCE:-<全部>} until=${UNTIL:-<全部>} only=${ONLY:-<全部>} max_partitions=${MAX_PARTITIONS:-<不限>} sleep=${SLEEP}s"
echo

partitions="$(run_sql "$list_partitions_sql")"
[ -n "$partitions" ] || { echo "没有查到任何分区（检查 PSQL_CMD 是否指向正确的库）" >&2; exit 1; }

total_updated=0
processed=0
skipped=0
started_at=$(date +%s)

while IFS='|' read -r name lo hi; do
  [ -n "$name" ] || continue
  if [ -n "$ONLY" ] && [ "$ONLY" != "$name" ]; then continue; fi
  if [ -n "$SINCE" ] && [ "$hi" -le "$SINCE" ]; then skipped=$((skipped + 1)); continue; fi
  if [ -n "$UNTIL" ] && [ "$lo" -ge "$UNTIL" ]; then skipped=$((skipped + 1)); continue; fi
  if [ "$MAX_PARTITIONS" -gt 0 ] && [ "$processed" -ge "$MAX_PARTITIONS" ]; then break; fi

  t0=$(date +%s)
  if [ "$DRY_RUN" -eq 1 ]; then
    n="$(run_sql "$(count_sql_for "$lo" "$hi")")"
    verb="待更新"
  else
    n="$(run_sql "$(update_sql_for "$lo" "$hi")")"
    verb="已更新"
  fi
  t1=$(date +%s)
  total_updated=$((total_updated + ${n:-0}))
  processed=$((processed + 1))
  printf '%-58s [%s,%s)  %s %8s 行  (%ss)\n' "$name" "$lo" "$hi" "$verb" "${n:-0}" "$((t1 - t0))"

  if [ "$SLEEP" -gt 0 ]; then sleep "$SLEEP"; fi
done <<< "$partitions"

elapsed=$(( $(date +%s) - started_at ))
echo
echo "分区处理 $processed 个（跳过 $skipped 个），共 ${total_updated} 行，耗时 ${elapsed}s"
if [ "$DRY_RUN" -eq 1 ]; then
  echo "这是 dry-run：没有任何写入。确认影响面后加 --yes 正式跑。"
else
  echo "下一步：跑 ops/wincount_gas_reward/03_validate.sql 看逐分区 NULL 行数与 negative_rows。"
  echo "      回填刚跑完时负值是正常的：实测 98.6% 恰好 = -1（±1 attoFIL 取整边界，真值必为 0），"
  echo "      只有 ≤ -2 的那批（实测 1.4%）才是真偏差。"
  echo "      ⇒ 跑 ops/wincount_gas_reward/05_normalize_negatives.sql 归一化（-1 置 0、≤-2 置 NULL），"
  echo "        再用 06_validate_after_normalize.sql 断言 negative_rows = 0。"
fi
