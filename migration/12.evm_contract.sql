-- auto-generated definition
create table fevm.evm_transfers
(
    epoch         bigint,
    actor_id      varchar,
    actor_address varchar,
    balance       numeric,
    gas_cost      numeric,
    user_address  varchar,
    value         numeric,
    exit_code     numeric,
    method_name   varchar,
    message_cid   varchar
);

comment on column fevm.evm_transfers.balance is '实时余额';

comment on column fevm.evm_transfers.gas_cost is '总Gas消耗';

create index evm_actors_epoch_actor_id_index
    on fevm.evm_transfers (epoch, actor_id);

create unique index evm_transfers_epoch_actor_id_message_cid_uindex
    on fevm.evm_transfers (epoch, actor_id, message_cid);


-- auto-generated definition
create table fevm.evm_transfer_stats
(
    epoch              bigint,
    actor_id           varchar,
    interval           varchar,
    acc_transfer_count bigint,
    acc_user_count     bigint,
    acc_gas_cost       numeric
);

create index evm_actor_stats_epoch_actor_id_interval_index
    on fevm.evm_transfer_stats (epoch, actor_id, interval);


drop index fevm.evm_transfers_epoch_actor_id_message_cid_uindex;

create unique index evm_transfers_message_cid_uindex
    on fevm.evm_transfers (message_cid);

drop index fevm.evm_actor_stats_epoch_actor_id_interval_index;

create unique index evm_actor_stats_epoch_actor_id_interval_index
    on fevm.evm_transfer_stats (epoch, actor_id, interval);

alter table fevm.evm_transfer_stats
    add actor_balance numeric;

alter table fevm.evm_transfer_stats
    add actor_address varchar;

alter table fevm.evm_transfer_stats
    add contract_address varchar;

alter table fevm.evm_transfer_stats
    add contract_name varchar;