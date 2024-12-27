create schema fns;
create table fns.events
(
    epoch       bigint,
    cid         varchar,
    log_index   bigint,
    contract    varchar,
    event_name  varchar,
    topics      character varying[],
    data        varchar,
    removed     boolean,
    method_name varchar
);

create unique index events_epoch_cid_log_index_uindex
    on fns.events (epoch, cid, log_index);

create index events_epoch_contract_index
    on fns.events (epoch, contract);

create table fns.transfers
(
    epoch     bigint,
    provider  varchar,
    cid       varchar,
    log_index bigint,
    method    varchar,
    "from"    varchar,
    "to"      varchar,
    contract  varchar,
    token_id  varchar,
    item      varchar
);

create unique index transfers_epoch_cid_log_index_uindex
    on fns.transfers (epoch, cid, log_index);

create table fns.tokens
(
    name             varchar,
    provider         varchar,
    token_id         varchar,
    node             varchar,
    registrant       varchar,
    controller       varchar,
    expired_at       bigint,
    last_event_epoch bigint,
    fil_address      varchar
);

create unique index tokens_name_uindex
    on fns.tokens (name, provider);

create table fns.actions
(
    epoch    bigint,
    name     varchar,
    provider varchar,
    action   integer,
    constraint actions_pk
        unique (epoch, name, provider)
);

comment on column fns.actions.action is '1 新增、2、更新、3  删除';