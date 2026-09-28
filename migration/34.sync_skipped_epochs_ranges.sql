-- 同步器跳过台账「区间跳」扩展：v1 只有「单高度跳过」，但 2026-09 主网实测的
-- 「节点侧历史状态不可用」缺口是**几万个连续高度**（一次 51,393 个），一个高度一个高度跳过毫无意义。
--
-- 本迁移给 chain.sync_skipped_epochs 增加三列，用于登记「一次跳过整段」的区间行：
--   error_class  —— 触发跳过的错误类别：data（数据级，历史行为）/ unrecoverable-state（节点侧历史状态不可用）
--   skipped_from / skipped_to —— 被跳过的高度区间 [skipped_from, skipped_to]（含两端）
--
-- 语义兼容（老记录与单高度记录 skipped_from/skipped_to 为 NULL，等同 skipped_from = skipped_to = epoch）：
--   * 单高度跳过：epoch = 该高度，skipped_from/skipped_to 为 NULL；
--   * 区间跳：epoch = skipped_from = 区间起点，skipped_to = 区间终点（含），
--     例如 epoch=6357058, skipped_from=6357058, skipped_to=6408839 表示这 51,782 个高度被一次跳过。
--
-- 部署注意：本文件必须先于代码上线前执行（表/列缺失时同步器会拒绝跳过并打 ERROR —— 宁可继续重试，
-- 也不做没有留痕的跳过）。幂等：可重复执行。
--
-- 【怎么把区间跳过的欠账补回来（可逆）】
--   1. 查被跳过的区间：
--        select syncer, epoch, skipped_from, skipped_to, failures, last_failed_at, left(error_message, 120)
--        from chain.sync_skipped_epochs
--        where syncer='chain' and skipped_to is not null order by epoch;
--   2. 确认节点历史状态已恢复（lotus 能按该高度取到状态树；聚合器 traces 正常）；
--   3. 把同步器进度指针改回区间起点并重启该同步器（同步器 Init 会以该值 +1 作为起点）：
--        update chain.sync_syncers set epoch = (select min(skipped_from) from chain.sync_skipped_epochs
--          where syncer='chain' and skipped_to is not null) - 1 where name='chain';
--        # 再重启 filscan-syncer（supervisorctl restart filscan-syncer）
--   4. 重跑成功（该区间的任务/计算器记录写回 chain.sync_task_epochs）后，删掉对应区间行：
--        delete from chain.sync_skipped_epochs
--        where syncer='chain' and skipped_from=? and skipped_to=?;
--   （重跑期间区间行仍被一致性检查当作「已知跳过」，不会因为任务缺失触发误判回滚。）

alter table chain.sync_skipped_epochs
    add column if not exists error_class  varchar, -- data / unrecoverable-state（NULL 等同 data）
    add column if not exists skipped_from bigint,  -- 区间跳：被跳过区间的起点（= epoch）
    add column if not exists skipped_to   bigint; -- 区间跳：被跳过区间的终点（含）

comment on column chain.sync_skipped_epochs.error_class is
    '触发跳过的错误类别：data（数据级）/ unrecoverable-state（节点侧历史状态不可用）；NULL 等同 data';
comment on column chain.sync_skipped_epochs.skipped_from is
    '区间跳：被跳过区间的起点（含），此时 epoch = skipped_from；单高度记录为 NULL';
comment on column chain.sync_skipped_epochs.skipped_to is
    '区间跳：被跳过区间的终点（含）；单高度记录为 NULL（等同 epoch）';

-- 按区间查询「欠账」用（GetSkippedEpochs 的区间求交条件：epoch <= ? and coalesce(skipped_to, epoch) >= ?）
create index if not exists sync_skipped_epochs_range_index
    on chain.sync_skipped_epochs (syncer, skipped_to);
