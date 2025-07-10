create table fevm.defi_dashboard(
    id  serial  not null primary key  ,
    epoch int not null,
    protocol varchar,
    contract_id varchar,
    tvl numeric,
    tvl_in_fil numeric,
    users int,
    url varchar
);