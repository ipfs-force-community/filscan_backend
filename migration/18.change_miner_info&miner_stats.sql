create index miner_infos_epoch_balance_index
    on chain.miner_infos (epoch, balance);

create index miner_stats_epoch_wining_rate_index
    on chain.miner_stats (epoch, wining_rate);

create index miner_stats_epoch_quality_adj_power_change_index
    on chain.miner_stats (epoch, quality_adj_power_change);

create index owner_stats_epoch_acc_block_count_index
    on chain.owner_stats (epoch, acc_block_count);