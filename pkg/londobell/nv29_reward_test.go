package londobell

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

// NV29(Solstice) 起，奖励 actor(f02) 的 TotalStoragePowerReward 改名为 TotalMintedReward，
// 且语义从「矿工出块奖励」变为「f02 全部铸造量 = 销毁 + 显式流 + 矿工份额」。
// 下面两份是升级前后从链上取回的真实状态片段（calibnet 升级高度 4109133）。
const (
	rewardStateOldSchema = `{"Epoch":4107459,"ThisEpochReward":"23095629521969828224","TotalStoragePowerReward":"110469517489432681170003963"}`
	rewardStateNewSchema = `{"Epoch":4110339,"ThisEpochReward":"23088309121363800201","TotalMintedReward":"110534789174400147324658484","TotalBurnMinted":"3338","TotalExplicitMinted":"1664003218449871124998"}`
)

// 老结构：TotalStoragePowerReward 本身就是矿工份额
const wantOldMinerMinted = "110469517489432681170003963"

// 新结构：矿工份额 = TotalMintedReward − TotalBurnMinted − TotalExplicitMinted
const wantNewMinerMinted = "110533125171181697453530148"

func TestRewardActorStateNV29MinerMinted(t *testing.T) {
	oldState := RewardActorState{}
	if err := json.Unmarshal([]byte(rewardStateOldSchema), &oldState); err != nil {
		t.Fatalf("unmarshal old schema: %s", err)
	}
	if got := oldState.MinerMinted(); got.String() != wantOldMinerMinted {
		t.Fatalf("老结构矿工份额错: got %s want %s", got, wantOldMinerMinted)
	}

	newState := RewardActorState{}
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &newState); err != nil {
		t.Fatalf("unmarshal new schema: %s", err)
	}
	if !newState.TotalStoragePowerReward.IsZero() {
		t.Fatalf("前置条件不成立：NV29 新结构不该有 TotalStoragePowerReward")
	}
	if got := newState.MinerMinted(); got.String() != wantNewMinerMinted {
		t.Fatalf("新结构矿工份额错（必须扣掉销毁与显式流）: got %s want %s", got, wantNewMinerMinted)
	}
	// 关键回归点：不得直接把 TotalMintedReward 当矿工份额（会高估）
	if newState.MinerMinted().Equal(newState.TotalMintedReward) {
		t.Fatalf("矿工份额被直接取成 TotalMintedReward（高估）")
	}
}

// 首页「近24h出块奖励」= MinerMinted(现在) − MinerMinted(24h前)：必须为正，且已扣减 burn/explicit
func TestRewardIncrease24HRegression(t *testing.T) {
	oldState := RewardActorState{}
	if err := json.Unmarshal([]byte(rewardStateOldSchema), &oldState); err != nil {
		t.Fatalf("unmarshal old: %s", err)
	}
	newState := RewardActorState{}
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &newState); err != nil {
		t.Fatalf("unmarshal new: %s", err)
	}

	delta := newState.MinerMinted().Sub(oldState.MinerMinted())
	if !delta.GreaterThan(decimal.Zero) {
		t.Fatalf("近24h出块奖励应为正, got %s", delta)
	}
	// 63607.68 FIL = 63607681749016283526185 attoFIL
	if delta.String() != "63607681749016283526185" {
		t.Fatalf("24h 增量与预期不符, got %s", delta)
	}
	// 必须小于「直接用 TotalMintedReward 求差」的高估值
	naive := newState.TotalMintedReward.Sub(oldState.MinerMinted())
	if !delta.LessThan(naive) {
		t.Fatalf("未扣减 burn/explicit：delta=%s 应小于 %s", delta, naive)
	}
}

func TestRewardActorDetailNV29MinerMinted(t *testing.T) {
	detail := RewardActorDetail{}
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &detail); err != nil {
		t.Fatalf("unmarshal: %s", err)
	}
	if got := detail.MinerMinted(); got.String() != wantNewMinerMinted {
		t.Fatalf("RewardActorDetail 矿工份额错: got %s", got)
	}
}
