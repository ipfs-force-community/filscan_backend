package londobell

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

// RewardStream 是 contract A (/aggregators/reward_streams) 的解码目标：
// 四字段恒定存在（缺失项输出 "0"），v18 只有 TotalStoragePowerReward，v19 只有
// TotalMintedReward/TotalBurnMinted/TotalExplicitMinted。
//
// 这里复用 nv29_reward_test.go 里**真实抓取**的 calibnet 状态片段
// （升级高度 4109133），确保两端都是链上真值而不是编造数字。
func TestRewardStreamContractADecode(t *testing.T) {
	// v18 行（真实样本）：contract A 会把缺失的三项补成 "0"。
	const v18Row = `{"Epoch":4107459,"TotalStoragePowerReward":"110469517489432681170003963","TotalMintedReward":"0","TotalBurnMinted":"0","TotalExplicitMinted":"0"}`
	var oldRow RewardStream
	if err := json.Unmarshal([]byte(v18Row), &oldRow); err != nil {
		t.Fatalf("解码 v18 行失败: %s", err)
	}
	if got := oldRow.MinerMinted(); got.String() != wantOldMinerMinted {
		t.Fatalf("v18 行矿工实收错: got %s want %s", got, wantOldMinerMinted)
	}

	// v19 行（真实样本）：TotalStoragePowerReward 应为 "0"。
	const v19Row = `{"Epoch":4110339,"TotalStoragePowerReward":"0","TotalMintedReward":"110534789174400147324658484","TotalBurnMinted":"3338","TotalExplicitMinted":"1664003218449871124998"}`
	var newRow RewardStream
	if err := json.Unmarshal([]byte(v19Row), &newRow); err != nil {
		t.Fatalf("解码 v19 行失败: %s", err)
	}
	if !newRow.TotalStoragePowerReward.IsZero() {
		t.Fatalf("v19 行不该有 TotalStoragePowerReward")
	}
	if got := newRow.MinerMinted(); got.String() != wantNewMinerMinted {
		t.Fatalf("v19 行矿工实收错: got %s want %s", got, wantNewMinerMinted)
	}
	// 关键回归：v19 不得把 TotalMintedReward 直接当矿工实收（会高估）。
	if newRow.MinerMinted().Equal(newRow.TotalMintedReward) {
		t.Fatal("v19 矿工实收被直接取成 TotalMintedReward（高估）")
	}
}

// 全字段为 0（真实存在的形态：升级瞬间/空计数）⇒ 矿工实收 0，不 panic。
func TestRewardStreamAllZero(t *testing.T) {
	var zero RewardStream
	if !zero.MinerMinted().IsZero() {
		t.Fatalf("全零行矿工实收应为 0，得到 %s", zero.MinerMinted())
	}
	// minted 有值但 burn+explicit 恰好等于 minted ⇒ 矿工实收 0。
	edge := RewardStream{
		TotalMintedReward:   decimal.RequireFromString("100000000000000000000"),
		TotalBurnMinted:     decimal.RequireFromString("40000000000000000000"),
		TotalExplicitMinted: decimal.RequireFromString("60000000000000000000"),
	}
	if !edge.MinerMinted().IsZero() {
		t.Fatalf("矿工实收应为 0，得到 %s", edge.MinerMinted())
	}
}

// 与 RewardActorDetail 同一套口径：同源样本必须得到完全相同的矿工实收。
func TestRewardStreamMinermintedMatchesRewardActorDetail(t *testing.T) {
	var stream RewardStream
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &stream); err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	var detail RewardActorDetail
	if err := json.Unmarshal([]byte(rewardStateNewSchema), &detail); err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	if !stream.MinerMinted().Equal(detail.MinerMinted()) {
		t.Fatalf("RewardStream 与 RewardActorDetail 口径分叉: %s vs %s",
			stream.MinerMinted(), detail.MinerMinted())
	}
}
