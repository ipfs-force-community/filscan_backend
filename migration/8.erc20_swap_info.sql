create table fevm.erc20_swap_info
(
    cid varchar,
    action varchar,
    epoch int,
    amount_in numeric,
    amount_out numeric,
    amount_in_token_name varchar,
    amount_out_token_name varchar,
    amount_in_contract_id varchar,
    amount_out_contract_id varchar,
    amount_in_decimal int,
    amount_out_decimal int,
    dex varchar,
    swap_rate numeric
);

alter table fevm.erc20_swap_info add values numeric;