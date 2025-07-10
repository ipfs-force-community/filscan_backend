create table public.fil_price(
    id  serial  not null primary key,
    timestamp int,
    price double precision,
    percent_change_24h double precision
);

