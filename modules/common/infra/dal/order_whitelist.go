package dal

import (
	"fmt"
	"strings"
)

// 排序白名单集中管理（ORDER BY 注入根治）。
//
// 事故背景：历史上 dal_evm_transfer.go / dal_evm_transaction.go 直接把请求里的
// field/sort 拼进 ORDER BY（fmt.Sprintf("ORDER BY %s %s", filed, sort)），
// dal_biz_miner_rank.go / dal_biz_owner_rank.go 则用模板变量 {{ SortField }} /
// {{ SortOrder }} 渲染。任何「用户可控字符串」直接进入 ORDER BY 都是 SQL 注入：
// PostgreSQL 的报错型注入可经 cast(('x'||pg_read_file('/etc/passwd'))::int) 读取数据库
// 主机上的任意文件并执行系统命令。
//
// 这里把所有站点的「对外字段名 -> 真实列名/表达式」收敛为 map[string]string 白名单：
//   - 请求里的原始字符串永不进入 SQL，只有白名单里刻死的列名才会被拼接；
//   - 未命中白名单的 field 直接返回错误（fail-closed，选择「报错」而非「静默回退」的理由见
//     buildOrderClause 注释）—— 任何未白名单化字符串都不可能进入 SQL，且探测行为会被显式暴露；
//   - sort 只接受 asc/desc（大小写不敏感、允许首尾空白），其余（含空值）回退到该站点的默认方向。

// 键 = 前端/请求里使用的对外字段名（PagingOrder.Field / EvmContractRequest.Field）；
// 值 = 真实列名或 SQL 表达式。所有值都是本文件里刻死的常量。
var (
	// fevm.evm_transfer_stats 合约转账统计列表（dal_evm_transfer.go GetEvmTransferStatsList）
	evmTransferStatsOrderColumns = map[string]string{
		"transfer_count": "acc_transfer_count",
		"user_count":     "acc_user_count",
		"gas_cost":       "acc_gas_cost",
		"actor_balance":  "actor_balance",
	}

	// 合约转账聚合列表（dal_evm_transfer.go GetEvmTransferList）——列名取自该查询的 SELECT 别名
	evmTransferListOrderColumns = map[string]string{
		"transfer_count": "transfer_count",
		"user_count":     "user_count",
		"gas_cost":       "gas_cost",
		"actor_balance":  "actor_balance",
		"contract_name":  "contract_name",
		"actor_address":  "actor_address",
	}

	// fevm.evm_transaction_stats 合约交易统计列表（dal_evm_transaction.go GetEvmTransactionStatsList）
	evmTransactionStatsOrderColumns = map[string]string{
		"transaction_count": "acc_transaction_count",
		"user_count":        "acc_user_count",
		"gas_cost":          "acc_gas_cost",
		"actor_balance":     "actor_balance",
	}

	// chain.miner_infos / chain.miner_stats 节点排行榜（dal_biz_miner_rank.go GetMinerRanks）
	minerRankOrderColumns = map[string]string{
		"quality_adj_power":  "a.quality_adj_power",
		"power_increase_24h": "b.quality_adj_power_change",
		"block_count":        "b.acc_block_count",
		"rewards":            "b.acc_reward",
		"balance":            "a.balance",
	}

	// 节点增速排行（dal_biz_miner_rank.go GetMinerPowerRanks）
	minerPowerRankOrderColumns = map[string]string{
		"power_ratio":            "quality_adj_power_change",
		"quality_power_increase": "quality_adj_power_change",
		"quality_adj_power":      "quality_adj_power",
		"raw_power":              "raw_byte_power",
	}

	// 节点收益排行（dal_biz_miner_rank.go GetMinerRewardRanks）
	minerRewardRankOrderColumns = map[string]string{
		"rewards":           "a.acc_reward",
		"block_count":       "a.acc_block_count",
		"winning_rate":      "a.wining_rate",
		"quality_adj_power": "b.quality_adj_power",
	}

	// Owner 排行榜（dal_biz_owner_rank.go GetOwnerRanks）
	ownerRankOrderColumns = map[string]string{
		"quality_adj_power": "a.quality_adj_power",
		"rewards_ratio_24h": "b.reward_power_ratio",
		"power_change_24h":  "b.quality_adj_power_change",
		"block_count":       "b.acc_block_count",
	}
)

// resolveOrder 把请求的 field/sort 解析为白名单内的列与方向。
//
//   - field 为空 -> 返回 defaultColumn（保持「未传排序」时的原有默认排序）；
//   - field 非空但不在 whitelist -> 返回错误，绝不回退到原始输入，也绝不让它进入 SQL；
//   - sort 仅接受 asc/desc（大小写不敏感、允许首尾空白），其余（含空）回退到 defaultSort。
//
// 返回的 column 恒为 whitelist 的值或 defaultColumn（本文件里的编译期常量）。
func resolveOrder(field, sort string, whitelist map[string]string, defaultColumn, defaultSort string) (column, direction string, err error) {
	column = defaultColumn
	if field != "" {
		col, ok := whitelist[field]
		if !ok {
			return "", "", fmt.Errorf("unsupported order field: %q", field)
		}
		column = col
	}
	direction = resolveSort(sort, defaultSort)
	return column, direction, nil
}

// resolveSort 归一化排序方向：asc/desc（大小写不敏感、允许首尾空白）归一为大写关键字，
// 其它任何取值（含空串、注入载荷）一律回退到 defaultSort，因此非法输入不会进入 SQL。
func resolveSort(sort, defaultSort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "asc":
		return "ASC"
	case "desc":
		return "DESC"
	default:
		return normalizeDirection(defaultSort)
	}
}

// normalizeDirection 把调用方给出的默认方向（编译期常量）归一为大写关键字。
// 只识别 asc/desc，其它值兜底为 DESC，保证即使调用方写错也只会产生合法关键字。
func normalizeDirection(defaultSort string) string {
	if strings.EqualFold(strings.TrimSpace(defaultSort), "asc") {
		return "ASC"
	}
	return "DESC"
}

// buildOrderClause 构造 "ORDER BY <白名单列> <方向>\n"。
//
// field 未命中白名单时返回错误而非「静默回退到默认排序」：
// 默认回退同样安全（不会注入），但它会把一次恶意的/损坏的排序请求伪装成正常响应，
// 掩盖攻击探测与前端回归；fail-closed 报错更保守，也与排行榜 dal 既有的
// "unsupported order field" 行为一致。因此这里选择返回明确错误。
func buildOrderClause(field, sort string, whitelist map[string]string, defaultColumn, defaultSort string) (string, error) {
	column, direction, err := resolveOrder(field, sort, whitelist, defaultColumn, defaultSort)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ORDER BY %s %s\n", column, direction), nil
}
