-- ops/wincount_gas_reward/06_validate_after_normalize.sql
--
-- 负值归一化（05_normalize_negatives.sql）之后的**硬断言 + 三组计数**。
--
-- 用法：
--   /root/ltr_pg.sh -At -F'|' -f ops/wincount_gas_reward/06_validate_after_normalize.sql
--
-- 退出码语义：断言失败 ⇒ `raise exception` ⇒ psql 退出码 3（可挂进巡检 / 上线门）。
--   ⚠ 前提是本文件顶部的 `\set ON_ERROR_STOP on` 生效 —— **psql 只在 ON_ERROR_STOP 打开时
--     才会因脚本内的错误返回非 0**（实测：不带它时，断言失败仍然退出 0）。所以：
--     * 必须用 `-f` 执行（`-c` 会把 `\set` 当普通文本）；
--     * 若 /root/ltr_pg.sh 这类包装脚本吞掉了退出码，就按输出里的
--       `ASSERT OK` / `ASSERT FAILED` 行判断（下面每段都会显式打印结论）。
--
-- 成本：A/B 两段各扫一次全表（2026-09-30 生产 16,448,363 行，单次扫描秒~十几秒级）。
--   要更省，把两段合成一段（B 段已经带了 negative_rows 的计数），或改用
--   03_validate.sql 的逐分区/估算版本。
--
\set ON_ERROR_STOP on

-- ============================================================
-- A. 硬断言：negative_rows 必须为 0
-- ============================================================
-- 判据：归一化后全表不允许再有任何 gas_reward < 0。
--   非 0 意味着：归一化没跑全（比如分区是在归一化之后才被回填的），
--   或者回填公式的模型被打破（penalty > 0 / 奖励 actor 余额不足）产生了新的真偏差
--   ⇒ 这批行不可信，必须重跑 05_normalize_negatives.sql（幂等）。
do $$
declare neg bigint;
begin
    select count(*) into neg from chain.miner_win_counts where gas_reward < 0;
    if neg <> 0 then
        raise exception 'ASSERT FAILED: negative_rows=% （期望 0）⇒ 重跑 05_normalize_negatives.sql', neg;
    end if;
    raise notice 'ASSERT OK: negative_rows=0';
end $$;

-- ============================================================
-- B. 三组计数：已回填 / NULL（回落聚合器）/ 合法零值
-- ============================================================
-- 判据（2026-09-30 生产实测基线，对得上就说明归一化按预期收口）：
--   total_rows      = 16,448,363
--   backfilled_rows = 14,117,903   （count(gas_reward)）
--   null_rows       =  2,330,460   （读路径会为这些区间**整请求回落聚合器**）
--   zero_rows       =  4,971,087   （合法值 0，含归一化置 0 的 1,987,583 行）
--   negative_rows   =          0
-- 注意：zero_rows ⊆ backfilled_rows；backfilled + null = total 必须成立。
select count(*)                                as total_rows,
       count(gas_reward)                       as backfilled_rows,
       count(*) - count(gas_reward)            as null_rows,
       count(*) filter (where gas_reward = 0)  as zero_rows,
       count(*) filter (where gas_reward < 0)  as negative_rows
from chain.miner_win_counts;

-- ============================================================
-- C. 一致性断言：分组必须闭合
-- ============================================================
-- 判据：
--   1) backfilled + null == total            （每个 (epoch,miner) 行恰好属于一组）
--   2) zero <= backfilled                    （0 是已回填值的一种）
--   3) null 行数与「读路径是否回落」的关系：NULL = 0 ⇒ 全区间走 PG；
--      NULL > 0 ⇒ 这些区间回落聚合器（**设计行为**，不是错值）。
-- 附带解释（2026-09-30 生产实测）：
--   null_rows 2,330,460 = 28,013（05 归一化置 NULL 的真偏差）
--                       + 2,302,447（未回填 / 未覆盖高度，含部署前刚同步的高度）
--   差值可自检：2,330,460 − 28,013 = 2,302,447
do $$
declare tot bigint; bf bigint; nu bigint; zr bigint; pct numeric;
begin
    select count(*), count(gas_reward), count(*) filter (where gas_reward is null),
           count(*) filter (where gas_reward = 0)
      into tot, bf, nu, zr
    from chain.miner_win_counts;

    if bf + nu <> tot then
        raise exception 'ASSERT FAILED: backfilled(%) + null(%) <> total(%)', bf, nu, tot;
    end if;
    if zr > bf then
        raise exception 'ASSERT FAILED: zero_rows(%) > backfilled_rows(%)', zr, bf;
    end if;
    pct := round(100.0 * nu / nullif(tot, 0), 2);
    raise notice 'ASSERT OK: total=% backfilled=% null=% zero=% null_pct=%',
        tot, bf, nu, zr, pct;
    if nu > 0 then
        raise notice 'NOTE: null_rows>0 ⇒ 这些区间读路径回落聚合器（设计行为，非错值）';
    end if;
end $$;

-- ============================================================
-- D. 记账自检：本次归一化的两条语句各改了多少（把上面的计数对上账）
-- ============================================================
-- 归一化是单向的（-1 → 0 之后与「真值本来就是 0」不可区分），所以这里只能核对
-- **总量**是否闭合，不能事后拆出「哪 1,987,583 行原来是 -1」。对账公式：
--
--   回填写入行数 − 置 NULL 行数 = 非 NULL 行数
--   14,145,916   −    28,013    = 14,117,903   ✓（= B 段的 backfilled_rows）
--
-- 回填写入行数（02_backfill.sql 的 dry-run 汇总，或 04_dryrun_premigration.sql）：
--   /root/ltr_pg.sh -At -F'|' -c "select count(*) from chain.miner_win_counts where gas_reward is not null"  -- 与 14,117,903 比
--   ⚠ 这个数只会**变大**（新同步器持续前向写入新高度），所以对账要在归一化刚跑完时做。

-- ============================================================
-- E. 巡检项（日常只需看这一条）
-- ============================================================
-- 只有 B 段的 negative_rows 需要常态化盯：非 0 ⇒ 重跑 05（幂等）。
-- NULL 行数非 0 是**预期**的（未回填高度 / 新分区），不必告警；
-- 想消掉它就跑 backfill.sh（幂等，只补 NULL）。
