alter table fevm.contracts rename actor_robust to actor_address;
alter table fevm.contracts rename contract to contract_address;
alter table fevm.contracts rename parameters to arguments;
alter table fevm.contracts rename "optimizeRuns" to optimize_runs;
alter table fevm.contracts rename authed to verified;
alter table fevm.contract_sols rename filename to file_name;
alter table fevm.contract_sols add byte_code text;
alter table fevm.contract_sols add abi text;
alter table fevm.contract_sols add contract_name varchar;
