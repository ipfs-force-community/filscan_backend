#!/usr/bin/env bash
#
# ops/wincount_gas_reward/dryrun_premigration.sh
#
# **migration/36 之前** 的回填影响面估算（纯只读，绝不发写）。
#
# 与 backfill.sh --dry-run 的区别：不引用 gas_reward 列（该列要 DDL 之后才存在），
# 改用 04_dryrun_premigration.sql（等价性证明见该文件头部）。DDL 之前用它估影响面；
# DDL 之后请用 backfill.sh --dry-run（它按 02_backfill.sql 同口径，会校验列已存在）。
#
# 用法：
#   export PSQL_CMD=/root/ltr_pg.sh
#   ops/wincount_gas_reward/dryrun_premigration.sh                 # 全量估算（317 个分区）
#   ops/wincount_gas_reward/dryrun_premigration.sh --max-partitions 3   # 先量耗时
#
# 输出：每分区一行「分区名 [lo,hi) 待更新 N 行 (Xs)」+ 末尾汇总（分区数 / 总行数 / 耗时）。

set -euo pipefail

PSQL_CMD="${PSQL_CMD:-psql}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="$HERE/04_dryrun_premigration.sql"

MAX_PARTITIONS=0
SLEEP=0
while [ $# -gt 0 ]; do
  case "$1" in
    --max-partitions) MAX_PARTITIONS="$2"; shift 2 ;;
    --sleep)          SLEEP="$2"; shift 2 ;;
    --help|-h)        sed -n '2,18p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

[ -f "$SQL_FILE" ] || { echo "找不到 $SQL_FILE" >&2; exit 1; }

run_sql() { $PSQL_CMD -At -F'|' -c "$1"; }

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

echo "PSQL_CMD = $PSQL_CMD"
echo "模式     = dry-run（只读估算，不发写）"
echo "SQL_FILE = $SQL_FILE"
echo

partitions="$(run_sql "$list_partitions_sql")"
[ -n "$partitions" ] || { echo "没有查到任何分区（检查 PSQL_CMD）" >&2; exit 1; }
total_partitions=$(printf '%s\n' "$partitions" | grep -c .)

total=0
processed=0
touched=0
started_at=$(date +%s)

while IFS='|' read -r name lo hi; do
  [ -n "$name" ] || continue
  if [ "$MAX_PARTITIONS" -gt 0 ] && [ "$processed" -ge "$MAX_PARTITIONS" ]; then break; fi
  t0=$(date +%s)
  sql="$(sed -e "s/:lo/$lo/g" -e "s/:hi/$hi/g" "$SQL_FILE")"
  n="$(run_sql "$sql")"
  t1=$(date +%s)
  n="${n:-0}"
  total=$((total + n))
  processed=$((processed + 1))
  [ "$n" -gt 0 ] && touched=$((touched + 1))
  printf '%-58s [%s,%s)  待更新 %8s 行  (%ss)\n' "$name" "$lo" "$hi" "$n" "$((t1 - t0))"
  if [ "$SLEEP" -gt 0 ]; then sleep "$SLEEP"; fi
done <<< "$partitions"

elapsed=$(( $(date +%s) - started_at ))
echo
echo "分区处理 $processed/$total_partitions 个（其中有待更新行的 $touched 个）"
echo "预计影响：$total 行，耗时 ${elapsed}s"
echo "这是估算：没有任何写入。确认影响面后，先跑 migration/36，再用 backfill.sh --yes 正式回填。"
