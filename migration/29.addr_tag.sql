create TABLE chain.addr_tags (
  id serial primary key,
  address varchar(1000),
  tag varchar(255)
);