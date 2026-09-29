package calc_change_actor_task

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// buildActorActions 的语义：库里已有 = Update，否则 = New；且输出顺序必须与入参（已按 Id 排序）一致，
// 以保证并发写库时加锁顺序稳定（降低 PostgreSQL 死锁概率）。
func TestBuildActorActions(t *testing.T) {
	epoch := chain.Epoch(4110180)

	actors := []*po.ActorPo{
		{Id: "t0100"},
		{Id: "t0200"},
		{Id: "t0300"},
	}
	oldMap := map[string]*po.ActorPo{
		"t0100": {Id: "t0100"}, // 已有 → Update
		"t0300": {Id: "t0300"}, // 已有 → Update
	}

	ids, actions := buildActorActions(epoch, actors, oldMap)

	require.Equal(t, []string{"t0100", "t0200", "t0300"}, ids, "输出顺序应与入参一致")
	require.Len(t, actions, 3)
	require.Equal(t, po.ActorActionUpdate, actions[0].Action, "库里已有应为 Update")
	require.Equal(t, po.ActorActionNew, actions[1].Action, "库里没有应为 New")
	require.Equal(t, po.ActorActionUpdate, actions[2].Action, "库里已有应为 Update")
	for i, a := range actions {
		require.Equal(t, epoch.Int64(), a.Epoch)
		require.Equal(t, ids[i], a.ActorId)
	}

	// 空输入不应 panic，也不应产生记录
	ids2, actions2 := buildActorActions(epoch, nil, nil)
	require.Empty(t, ids2)
	require.Empty(t, actions2)
}
