create schema chain;

create table chain.actor_balances
(
    epoch      bigint,
    actor_id   varchar,
    actor_type varchar,
    balance    numeric
) partition by RANGE (epoch);

CREATE UNIQUE INDEX actor_balances_epoch_actor_id_uindex ON chain.actor_balances USING btree (epoch, actor_id);

create table chain.actors
(
    id           varchar,
    robust       varchar,
    type         varchar,
    code         varchar,
    created_time timestamp,
    last_tx_time timestamp,
    balance      numeric
);

CREATE UNIQUE INDEX actors_id_uindex ON chain.actors USING btree (id);
CREATE UNIQUE INDEX actors_robust_uindex ON chain.actors USING btree (robust);
CREATE INDEX actors_type_balance_index ON chain.actors USING btree (type, balance);

create table chain.base_gas_costs
(
    epoch        bigint,
    base_gas     numeric,
    sector_gas32 numeric,
    sector_gas64 numeric,
    messages     bigint,
    acc_messages bigint
) partition by RANGE (epoch);


create table chain.builtin_actor_states
(
    epoch   bigint,
    actor   varchar,
    state   jsonb,
    balance numeric
) partition by RANGE (epoch);

CREATE UNIQUE INDEX builtin_actor_states_epoch_actor_uindex ON chain.builtin_actor_states USING btree (epoch, actor);

create table chain.deal_proposals
(
    epoch   bigint,
    cid     varchar,
    deal_id bigint
) partition by RANGE (epoch);

create table chain.message_counts
(
    epoch             bigint,
    message           bigint,
    avg_block_message bigint,
    block             bigint
) partition by RANGE (epoch);


create table chain.method_gas_fees
(
    epoch       bigint,
    method      varchar,
    count       bigint,
    gas_premium numeric,
    gas_limit   numeric,
    gas_cost    numeric,
    gas_fee     numeric
) partition by RANGE (epoch);

CREATE UNIQUE INDEX method_gas_fees_epoch_method_uindex ON chain.method_gas_fees USING btree (epoch, method);

create table chain.miner_gas_fees
(
    epoch       bigint,
    miner       varchar,
    pre_agg     numeric,
    prove_agg   numeric,
    sector_gas  numeric,
    seal_gas    numeric,
    wd_post_gas numeric
) partition by RANGE (epoch);


create table chain.miner_infos
(
    epoch                     bigint,
    miner                     varchar,
    owner                     varchar,
    worker                    varchar,
    controllers               character varying[],
    raw_byte_power            numeric,
    quality_adj_power         numeric,
    balance                   numeric,
    available_balance         numeric,
    vesting_funds             numeric,
    fee_debt                  numeric,
    sector_size               bigint,
    sector_count              bigint,
    fault_sector_count        bigint,
    active_sector_count       bigint,
    live_sector_count         bigint,
    recover_sector_count      bigint,
    terminate_sector_count    bigint,
    pre_commit_sector_count   bigint,
    initial_pledge            numeric,
    pre_commit_deposits       numeric,
    quality_adj_power_rank    numeric,
    quality_adj_power_percent numeric,
    ips                       character varying[]
) partition by RANGE (epoch);

CREATE UNIQUE INDEX miner_infos_epoch_miner_uindex ON chain.miner_infos USING btree (epoch, miner);
CREATE INDEX miner_infos_epoch_quality_adj_power_index ON chain.miner_infos USING btree (epoch, quality_adj_power);
CREATE INDEX miner_infos_epoch_quality_adj_power_rank_index ON chain.miner_infos USING btree (epoch, quality_adj_power_rank);


create table chain.miner_locations
(
    miner       varchar,
    country     varchar,
    city        varchar,
    region      varchar,
    latitude    double precision,
    longitude   double precision,
    updated_at  timestamp,
    ip          varchar,
    multi_addrs character varying[]
);

CREATE UNIQUE INDEX miner_locations_miner_uindex ON chain.miner_locations USING btree (miner);

create table chain.miner_reward_stats
(
    epoch            bigint,
    interval         varchar,
    acc_reward       numeric,
    acc_reward_per_t numeric
) partition by RANGE (epoch);


create table chain.miner_rewards
(
    epoch           bigint,
    miner           varchar,
    reward          numeric,
    block_count     bigint,
    block_time      timestamp,
    acc_reward      numeric,
    acc_block_count numeric,
    prev_reward_ref bigint
) partition by RANGE (epoch);

CREATE UNIQUE INDEX miner_rewards_epoch_miner_uindex ON chain.miner_rewards USING btree (epoch, miner);

create table chain.miner_stats
(
    epoch                    bigint,
    miner                    varchar,
    interval                 varchar,
    prev_epoch_ref           bigint,
    raw_byte_power_change    numeric,
    quality_adj_power_change numeric,
    initial_pledge_change    numeric,
    acc_reward               numeric,
    acc_block_count          bigint,
    acc_block_count_percent  numeric,
    acc_win_count            bigint,
    acc_seal_gas             numeric,
    acc_wd_post_gas          numeric,
    acc_reward_percent       numeric,
    sector_count_change      bigint,
    reward_power_ratio       numeric,
    wining_rate              numeric,
    luck_rate                numeric
) partition by RANGE (epoch);

comment
on column chain.miner_stats.reward_power_ratio is '单T收益';

CREATE INDEX miner_stats_epoch_miner_interval_index ON chain.miner_stats USING btree (epoch, miner, "interval");
CREATE INDEX miner_stats_epoch_acc_reward_index ON chain.miner_stats USING btree (epoch, acc_reward);
CREATE INDEX miner_stats_epoch_acc_block_count_index ON chain.miner_stats USING btree (epoch, acc_block_count);

create table chain.miner_win_counts
(
    epoch     bigint,
    miner     varchar,
    win_count bigint
) partition by RANGE (epoch);

CREATE  INDEX miner_win_counts_epoch_miner_index ON ONLY chain.miner_win_counts USING btree (epoch, miner);

CREATE  INDEX miner_win_counts_miner_epoch_index ON ONLY chain.miner_win_counts USING btree (miner,epoch);


create table chain.owner_infos
(
    epoch                     bigint,
    owner                     varchar,
    miners                    character varying[],
    raw_byte_power            numeric,
    quality_adj_power         numeric,
    balance                   numeric,
    available_balance         numeric,
    vesting_funds             numeric,
    fee_debt                  numeric,
    sector_size               bigint,
    sector_count              bigint,
    fault_sector_count        bigint,
    active_sector_count       bigint,
    live_sector_count         bigint,
    recover_sector_count      bigint,
    terminate_sector_count    bigint,
    pre_commit_sector_count   bigint,
    initial_pledge            numeric,
    pre_commit_deposits       numeric,
    quality_adj_power_percent numeric,
    quality_adj_power_rank    numeric
) partition by RANGE (epoch);


CREATE UNIQUE INDEX owner_infos_epoch_owner_uindex ON chain.owner_infos USING btree (epoch, owner);
CREATE INDEX owner_infos_epoch_quality_adj_power_index ON chain.owner_infos USING btree (epoch, quality_adj_power);
CREATE INDEX owner_infos_epoch_quality_adj_power_rank_index ON chain.owner_infos USING btree (epoch, quality_adj_power_rank);

create table chain.owner_rewards
(
    epoch           bigint,
    owner           varchar,
    reward          numeric,
    block_count     bigint,
    prev_epoch_ref  bigint,
    acc_reward      numeric,
    acc_block_count bigint,
    sync_miner_ref  bigint,
    miners          character varying[]
) partition by RANGE (epoch);

CREATE UNIQUE INDEX acc_owner_rewards_epoch_owner_uindex ON chain.owner_rewards USING btree (epoch, owner);


create table chain.owner_stats
(
    epoch                    bigint,
    owner                    varchar,
    interval                 varchar,
    prev_epoch_ref           bigint,
    raw_byte_power_change    numeric,
    quality_adj_power_change numeric,
    initial_pledge_change    numeric,
    acc_reward               numeric,
    acc_block_count          bigint,
    acc_block_count_percent  numeric,
    acc_win_count            bigint,
    acc_seal_gas             numeric,
    acc_wd_post_gas          numeric,
    acc_reward_percent       numeric,
    sector_count_change      bigint,
    reward_power_ratio       numeric
) partition by RANGE (epoch);

CREATE INDEX owner_stats_epoch_owner_interval_index ON chain.owner_stats USING btree (epoch, owner, "interval");
CREATE INDEX owner_stats_epoch_quality_adj_power_change_index ON chain.owner_stats USING btree (epoch, quality_adj_power_change);
CREATE INDEX owner_stats_epoch_acc_reward_index ON chain.owner_stats USING btree (epoch, acc_reward);
CREATE INDEX owner_stats_epoch_reward_power_ratio_index ON chain.owner_stats USING btree (epoch, reward_power_ratio);

create table chain.rich_actors
(
    epoch      bigint,
    actor_id   varchar,
    actor_type varchar,
    balance    numeric
);

CREATE UNIQUE INDEX actors_balance_epoch_actor_id_uindex ON chain.rich_actors USING btree (epoch, actor_id);

create table chain.sync_epochs
(
    epoch bigint,
    empty boolean,
    cost  interval,
    cids  character varying[]
) partition by RANGE (epoch);

CREATE UNIQUE INDEX sync_epochs_epoch_uindex ON chain.sync_epochs USING btree (epoch);


create table chain.sync_miner_epochs
(
    epoch            bigint,
    effective_miners bigint,
    owners           bigint
) partition by RANGE (epoch);


CREATE UNIQUE INDEX sync_miner_epochs_epoch_uindex ON chain.sync_miner_epochs USING btree (epoch);

create table chain.sync_tasks
(
    name  varchar not null
        constraint sync_tasks_pk
            primary key,
    epoch bigint
);

CREATE UNIQUE INDEX sync_tasks_name_uindex ON chain.sync_tasks USING btree (name);



call public.chain_partition_declare('chain', 'actor_balances');
call public.chain_partition_declare('chain', 'base_gas_costs');
call public.chain_partition_declare('chain', 'builtin_actor_states');
call public.chain_partition_declare('chain', 'deal_proposals');
call public.chain_partition_declare('chain', 'message_counts');
call public.chain_partition_declare('chain', 'method_gas_fees');
call public.chain_partition_declare('chain', 'miner_gas_fees');
call public.chain_partition_declare('chain', 'miner_infos');
call public.chain_partition_declare('chain', 'miner_reward_stats');
call public.chain_partition_declare('chain', 'miner_rewards');
call public.chain_partition_declare('chain', 'miner_stats');
call public.chain_partition_declare('chain', 'miner_win_counts');
call public.chain_partition_declare('chain', 'owner_infos');
call public.chain_partition_declare('chain', 'owner_rewards');
call public.chain_partition_declare('chain', 'owner_stats');
call public.chain_partition_declare('chain', 'sync_epochs');
call public.chain_partition_declare('chain', 'sync_miner_epochs');