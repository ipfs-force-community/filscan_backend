alter table chain.base_gas_costs
    add avg_gas_limit32 numeric;

alter table chain.base_gas_costs
    add avg_gas_limit64 numeric;

alter table chain.base_gas_costs
    add sector_fee32 numeric;

alter table chain.base_gas_costs
    add sector_fee64 numeric;