create table chain.abs_power_change(
    id  serial  not null primary key,
    epoch int,
    power_increase numeric ,
    power_loss numeric
);

create unique index abs_power_change_epoch_idx
    on chain.abs_power_change (epoch);