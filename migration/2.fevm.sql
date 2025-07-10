create schema fevm;

create table fevm.abi_signatures
(
    type varchar,
    id   varchar,
    name varchar,
    raw  varchar
);

CREATE UNIQUE INDEX abi_signatures_type_id_uindex ON fevm.abi_signatures USING btree (type, id);

create table fevm.contract_sols
(
    actor_id   varchar,
    filename   varchar,
    source     text,
    size       bigint,
    created_at timestamp with time zone default CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX contract_sols_actor_id_filename_uindex ON fevm.contract_sols USING btree (actor_id, filename);

create table fevm.contracts
(
    actor_id       varchar,
    actor_robust   varchar,
    contract       varchar,
    parameters     varchar,
    license        varchar,
    language       varchar,
    compiler       varchar,
    optimize       boolean,
    "optimizeRuns" bigint,
    authed         boolean,
    created_at     timestamp with time zone default CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX contracts_actor_id_uindex ON fevm.contracts USING btree (actor_id);

create table fevm.erc_20_transfers
(
    epoch       bigint,
    cid         varchar,
    contract_id varchar,
    "from"      varchar,
    "to"        varchar,
    amount      numeric
) partition by RANGE (epoch);

create table fevm.erc_721_tokens
(
    contract_id varchar,
    token_id    varchar,
    contract    varchar,
    name        varchar,
    symbol      varchar,
    token_uri   varchar,
    constraint erc_721_tokens_pk
        unique (contract_id, token_id)
);

create table fevm.erc_721_transfers
(
    epoch       bigint,
    cid         bigint,
    contract_id varchar,
    method      varchar,
    "from"      varchar,
    "to"        varchar,
    token_id    varchar
) partition by RANGE (epoch);


call public.chain_partition_declare('fevm', 'erc_721_transfers');
call public.chain_partition_declare('fevm', 'erc_20_transfers');