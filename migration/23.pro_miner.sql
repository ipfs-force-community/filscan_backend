create table chain.miner_agg_rewards
(
    miner         varchar not null
        constraint miner_agg_rewards_pk
            primary key,
    agg_reward    numeric,
    agg_block     bigint,
    agg_win_count bigint
);


create table pro.miner_balances
(
    epoch   bigint  not null,
    miner   varchar not null,
    type    varchar not null,
    balance numeric,
    address varchar,
    constraint miner_balances_pk
        primary key (epoch, miner, type)
);

create table pro.miner_dcs
(
    epoch             bigint  not null,
    miner             varchar not null,
    raw_byte_power    numeric,
    quality_adj_power numeric,
    pledge            numeric,
    live_sectors      bigint,
    active_sectors    bigint,
    fault_sectors     bigint,
    sector_size       bigint,
    vdc_power         numeric,
    dc_power          numeric,
    cc_power          numeric,
    constraint miner_dcs_pk
        primary key (epoch, miner)
)
    partition by RANGE (epoch);

create table pro.miner_funds
(
    epoch       bigint  not null,
    miner       varchar not null,
    income      numeric,
    outlay     numeric,
    seal_gas    numeric,
    wd_post_gas numeric,
    deal_gas    numeric,
    total_gas   numeric,
    penalty     numeric,
    reward      numeric,
    block_count bigint,
    win_count   bigint,
    other_gas   numeric,
    pre_agg     numeric,
    pro_agg     numeric,
    constraint miner_funds_pk
        primary key (epoch, miner)
)
    partition by RANGE (epoch);

create table pro.miner_infos
(
    epoch             bigint  not null,
    miner             varchar not null,
    raw_byte_power    numeric,
    quality_adj_power numeric,
    pledge            numeric,
    live_sectors      bigint,
    active_sectors    bigint,
    fault_sectors     bigint,
    sector_size       bigint,
    padding           boolean,
    worker            varchar,
    owner             varchar,
    controllers       character varying[],
    beneficiary       varchar,
    constraint miner_infos_pk
        primary key (epoch, miner)
)
    partition by RANGE (epoch);

create table pro.miner_sectors
(
    epoch      bigint  not null,
    miner      varchar not null,
    hour_epoch bigint  not null,
    sectors    bigint,
    power      numeric,
    pledge     numeric,
    vdc        numeric,
    dc         numeric,
    cc         numeric,
    constraint miner_sectors_pk
        primary key (epoch, miner, hour_epoch)
);

call public.chain_partition_declare('pro','miner_dcs');
call public.chain_partition_declare('pro','miner_funds');
call public.chain_partition_declare('pro','miner_infos');