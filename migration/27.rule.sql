create schema monitor;

CREATE TABLE monitor.rules
(
    id serial primary key,
    user_id    integer
        constraint rules_users_id_fk
            references pro.users
            on update cascade on delete cascade,
    group_id integer
        constraint rules_groups_id_fk
            references pro.groups
            on update cascade on delete cascade,
    miner_id_or_all varchar(16),
    uuid varchar(255), -- 用来将相关的字段放在一起，如balance下多个规则聚在一起
    account_type varchar(32),
    account_addr varchar(255),
    monitor_type varchar(32),
    operator varchar(3),
    operand varchar(255),
    mail_alert varchar(255),
    msg_alert varchar(255),
    call_alert varchar(255),
    interval integer,
    is_active boolean default true,
    is_vip boolean default true,
    description varchar(255),
    created_at timestamp with time zone default CURRENT_TIMESTAMP,
    updated_at timestamp with time zone
);

ALTER TABLE monitor.rules ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP;
-- 加入该默认值是为了SQL操作时候更方便的插入

create index rules_user_id_miner_id_index
    on monitor.rules (user_id, miner_id_or_all);

create index rules_user_id_group_id_monitor_type_account_type_index
    on monitor.rules (user_id, group_id, monitor_type, account_type);

create unique index rules_uid_monitor_type_gid_account_type_account_addr_uindex
    on monitor.rules (user_id, monitor_type, group_id, miner_id_or_all,account_type, account_addr);