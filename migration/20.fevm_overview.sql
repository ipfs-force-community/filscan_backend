create table public.fevm_item(
    id  serial  not null primary key  ,
    twitter varchar,
    main_site varchar,
    name varchar,
    logo varchar
);

create table public.fevm_item_category(
    id  serial  not null primary key  ,
    item_id int,
    category varchar,
    orders int
);

create table public.hot_fevm_items(
    id  serial  not null primary key  ,
    item_id int,
    category varchar,
    orders int
);