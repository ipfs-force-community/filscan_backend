alter table fevm.contract_sols
add is_main_contract bool;

comment on column fevm.contract_sols.is_main_contract is '是否是主合约';

drop index fevm.contract_sols_actor_id_filename_uindex;

create unique index contract_sols_actor_id_filename_uindex
on fevm.contract_sols (actor_id, file_name, contract_name);