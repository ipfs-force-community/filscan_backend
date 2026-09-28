package londobell

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

// NV29(Solstice) 起奖励 actor 的 TotalStoragePowerReward 改名为 TotalMintedReward。
// 这段测试用升级前后两份真实状态片段，锁住「解码后 24h 出块奖励不再是负数」这个回归点。
const (
	rewardStateOldSchema = `{"Epoch":4107459,"ThisEpochReward":"23095629521969828224","TotalStoragePowerReward":"110469517489432681170003963"}`
	rewardStateNewSchema = `{"Epoch":4110339,"ThisEpochReward":"23088309121363800201","TotalMintedReward":"110534789174400147324658484"}`
)

func TestRewardStateNV29Fallback(t *testing.T) {
	// 新结构：老字段缺失时应回填
	newState := RewardActorState{}
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &newState); err != nil {
		t.Fatalf("unmarshal new schema: %s", err)
	}
	if !newState.TotalStoragePowerReward.IsZero() {
		t.Fatalf("前置条件不成立：新结构不该有 TotalStoragePowerReward")
	}
	newState.NormalizeNV29()
	if newState.TotalStoragePowerReward.String() != "110534789174400147324658484" {
		t.Fatalf("回填失败, got %s", newState.TotalStoragePowerReward)
	}

	// 老结构：不应被改动
	oldState := RewardActorState{}
	if err := json.Unmarshal([]byte(rewardStateOldSchema), &oldState); err != nil {
		t.Fatalf("unmarshal old schema: %s", err)
	}
	oldState.NormalizeNV29()
	if oldState.TotalStoragePowerReward.String() != "110469517489432681170003963" {
		t.Fatalf("老结构被误改, got %s", oldState.TotalStoragePowerReward)
	}

	// 回归点：近24h出块奖励 = 现在 - 24h前，必须是正数（修复前为 -1.104e26）
	delta := newState.TotalStoragePowerReward.Sub(oldState.TotalStoragePowerReward)
	if !delta.GreaterThan(decimal.Zero) {
		t.Fatalf("近24h出块奖励应为正, got %s", delta)
	}
	// 期望 ≈ 6.5272e22 attoFIL
	if delta.String() != "65271684967466154654521" {
		t.Fatalf("24h 增量与预期不符, got %s", delta)
	}
}

func TestRewardActorDetailNV29Fallback(t *testing.T) {
	detail := RewardActorDetail{}
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &detail); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	detail.NormalizeNV29()
	if detail.TotalStoragePowerReward.String() != "110534789174400147324658484" {
		t.Fatalf("RewardActorDetail 回填失败, got %s", detail.TotalStoragePowerReward)
	}
}
