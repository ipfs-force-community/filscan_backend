create table fevm.dex_info
(
    contract_id varchar,
    dex_name varchar,
    dex_url varchar,
    icon_url varchar
);

create unique index dex_info_contract_id_idx on fevm.dex_info (contract_id);

CREATE INDEX "evm_unique_users" ON "fevm"."evm_transfers" USING btree (
  "user_address"
);