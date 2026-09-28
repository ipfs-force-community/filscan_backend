-- 同步器「数据级错误跳过台账」：记录因不可恢复的数据级错误（例如聚合器对某高度返回 code:1、
-- 响应体无法解码）被主动跳过的高度，供运维排查与重跑。
--
-- 背景：2026-09-10 起主网索引链因「同一高度数据不可解析」被无限重试 18 天（静默停摆），
-- 该表是「连续 N 次数据级失败即跳过并继续」的留痕位置（见 modules/syncer/syncer.go 与 data_error.go）。
--
-- 部署注意：本表必须先于代码上线前建好；表缺失时同步器在需要跳过时会拒绝跳过并打 ERROR
-- （宁可继续重试，也不做没有留痕的跳过）。

create table chain.sync_skipped_epochs
(
    id              bigserial primary key,
    syncer          varchar     not null, -- 同步器名称
    epoch           bigint      not null, -- 被跳过的高度
    error_message   text,                 -- 触发跳过的错误摘要（已截断）
    failures        bigint,               -- 触发跳过时的连续失败次数
    first_failed_at timestamptz,          -- 首次失败时间
    last_failed_at  timestamptz,          -- 末次失败时间
    created_at      timestamptz,          -- 首次登记时间
    updated_at      timestamptz           -- 最近写入时间
);

create unique index sync_skipped_epochs_syncer_epoch_uindex
    on chain.sync_skipped_epochs (syncer, epoch);
