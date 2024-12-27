create table fevm.erc20_balance
(
    owner varchar,
    contract_id varchar,
    amount numeric
);

CREATE UNIQUE INDEX unique_idx_erc20_balance ON fevm.erc20_balance USING btree (owner, contract_id);


ALTER TABLE "fevm"."erc_20_transfers"
    ADD column "index" int;
