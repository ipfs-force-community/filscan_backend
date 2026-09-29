package po

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归测试：MinerAggReward 的表名曾经写成 chain.miner_age_rewards（agg 拼成 age）。
// 库里只有 chain.miner_agg_rewards（migration/23.pro_miner.sql:1），一旦有人用
// gorm 的链式 API（tx.Create/Find/Scan 进该 po）就会打到一张不存在的表。
func TestMinerAggRewardTableName(t *testing.T) {
	require.Equal(t, "chain.miner_agg_rewards", MinerAggReward{}.TableName())
	require.NotEqual(t, "chain.miner_age_rewards", MinerAggReward{}.TableName(), "agg 不能拼成 age")
}

// 表名必须与建表脚本一致：把「po 里的字面量」钉在 migration 上，避免只改一处
func TestMinerAggRewardTableNameMatchesMigration(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "migration", "23.pro_miner.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("找不到建表脚本 %s: %v", path, err)
	}

	var created string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "create table ") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				created = fields[2]
			}
			break
		}
	}
	require.NotEmpty(t, created, "%s 里应能找到 create table 语句", path)
	require.Equal(t, created, MinerAggReward{}.TableName())
}
