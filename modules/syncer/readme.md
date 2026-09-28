# Syncer

用于日常链数据实时同步、补差历史数据等。

## 特性

1. 支持数据同步任务（并发）、数据计算任务（串行）；
2. 自动并发执行数据同步任务，在达到阈值后，串行执行已同步数据高度的数据计算任务；
3. 自动检查链数据回滚；
4. 支持 Dry 模式运行，期间不处理 Tipset Keys 记录，可用于补全历史数据；
5. 可多同步器运行，将链数据、合约数据、Actor变化数据分开同步，以降低快慢数据延迟对网站带来的影响。


> 注意：对落库数据有顺序依赖，比如计算 Miner 的算力变化；或者对记录数据有先后顺序要求，如记录富豪榜，需要用计算任务。

## 按高度的增量维护任务

- 大额转账增量（`chain.large_transfers`）：`chain/large-transfer-task`，开关 `[syncer] sync_large_transfers`（默认关闭）——
  干什么、为什么挂在这个同步器、口径与幂等/回滚口径、上线与验收见 `chain/large-transfer-task/README.md`。

## 用法:

参考: modules/syncer/syncer_test.go