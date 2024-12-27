create table chain.sync_syncers
(
    name  varchar,
    epoch bigint
);

create table chain.sync_task_epochs
(
    epoch  bigint,
    task   varchar,
    cost   bigint,
    syncer varchar
);

create unique index sync_task_epochs_epoch_task_uindex
    on chain.sync_task_epochs (epoch, task);

create table chain.sync_syncer_epochs
(
    epoch       bigint,
    empty       boolean,
    keys        character varying[],
    name        varchar,
    parent_keys character varying[],
    cost        bigint
);

create unique index sync_syncer_epochs_epoch_name_uindex
    on chain.sync_syncer_epochs (epoch, name);

alter table chain.actor_balances
    add prev_epoch bigint;
