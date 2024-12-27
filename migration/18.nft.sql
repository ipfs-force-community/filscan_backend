create table fevm.nft_tokens
(
    token_id  varchar not null,
    contract  varchar not null,
    name      varchar,
    symbol    varchar,
    token_uri varchar,
    token_url varchar,
    owner     varchar,
    item      varchar,
    constraint erc721_tokens_pk
        primary key (contract, token_id)
);

create index erc721_tokens_owner_index
    on fevm.nft_tokens (owner);

create table fevm.nft_contracts
(
    contract   varchar not null
        constraint erc721_contracts_pkey
            primary key
        constraint erc721_contracts_contract_key
            unique,
    collection varchar,
    type       varchar,
    owners     bigint,
    transfers  bigint,
    logo       varchar,
    mints      bigint
);

create index nft_contracts_owners_index
    on fevm.nft_contracts (owners);

create table fevm.nft_transfers
(
    epoch    bigint  not null,
    cid      varchar not null,
    contract varchar,
    method   varchar,
    "from"   varchar,
    "to"     varchar,
    token_id varchar,
    item     varchar,
    value    numeric,
    constraint nft_transfers_pk
        primary key (epoch, cid)
) partition by RANGE (epoch);

create index nft_transfers_contract_index
    on fevm.nft_transfers (contract);

call public.chain_partition_declare('fevm','nft_transfers');