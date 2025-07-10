create TABLE chain.release_addrs (
  id serial primary key,
  address varchar(1000),
  tag varchar(1000),
  daily_release float,
  start_epoch int,
  end_epoch int,
  initial_lock float
);

