create schema pro;

create table pro.users
(
    id            serial
        primary key,
    mail          varchar,
    created_at    timestamp with time zone default CURRENT_TIMESTAMP,
    password      varchar,
    last_login_at timestamp with time zone,
    login_at      timestamp with time zone,
    name          varchar
);

ALTER TABLE pro.users
    ADD COLUMN is_activity BOOLEAN DEFAULT false;

create unique index users_mail_uindex
    on pro.users (mail);


create table pro.groups
(
    id         serial
        constraint groups_pk
            primary key,
    user_id    integer
        constraint groups_users_id_fk
            references pro.users,
    group_name varchar,
    created_at timestamp with time zone default CURRENT_TIMESTAMP,
    updated_at timestamp with time zone,
    is_default boolean
);

create unique index groups_user_id_group_name_uindex
    on pro.groups (user_id, group_name);


create table pro.user_miners
(
    user_id    integer
        constraint user_miners_users_id_fk
            references pro.users
            on update cascade on delete cascade,
    group_id   integer
        constraint user_miners_groups_id_fk
            references pro.groups
            on update cascade on delete cascade,
    miner_id   varchar,
    miner_tag  varchar,
    created_at timestamp with time zone default CURRENT_TIMESTAMP,
    updated_at timestamp with time zone
);

create unique index user_miners_user_id_miner_id_uindex
    on pro.user_miners (user_id, miner_id);

create unique index user_miners_miner_id_group_id_uindex
    on pro.user_miners (miner_id, group_id);

