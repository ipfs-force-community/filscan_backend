package acl

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// 真实抓取的 calibnet f02 状态（升级高度 4109133 前后；见 pkg/londobell/nv29_reward_test.go）。
const (
	rewardTotalsV18RealRow = `{"Epoch":4107459,"TotalStoragePowerReward":"110469517489432681170003963","TotalMintedReward":"0","TotalBurnMinted":"0","TotalExplicitMinted":"0"}`
	rewardTotalsV19RealRow = `{"Epoch":4110339,"TotalStoragePowerReward":"0","TotalMintedReward":"110534789174400147324658484","TotalBurnMinted":"3338","TotalExplicitMinted":"1664003218449871124998"}`
)

func stateMap(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("构造样本失败: %s", err)
	}
	return m
}

// v18 分支：只有矿工实收一条线 —— Minted=Miner=TotalStoragePowerReward，service/burn 恒 0。
func TestGetRewardStreamTotalsV18(t *testing.T) {
	adapter := &fakeIndexAdapter{state: stateMap(t, rewardTotalsV18RealRow)}
	a := newTestAcl(&fakeIndexAgg{}, adapter, &fakeWinCountRepo{})

	totals, err := a.GetRewardStreamTotals(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if !totals.OK {
		t.Fatal("v18 取数成功应 OK=true")
	}
	want := decimal.RequireFromString("110469517489432681170003963")
	if !totals.Miner.Equal(want) {
		t.Fatalf("v18 矿工实收错: got %s want %s", totals.Miner, want)
	}
	if !totals.Minted.Equal(want) {
		t.Fatalf("v18 累计铸造量必须等于矿工实收（否则前端 minted 显示 0 与矿工行矛盾）: got %s", totals.Minted)
	}
	if !totals.Service.IsZero() || !totals.Burn.IsZero() {
		t.Fatalf("v18 service/burn 必须为 0: service=%s burn=%s", totals.Service, totals.Burn)
	}
	if adapter.calls != 1 {
		t.Fatalf("只应读 f02 一次，实际 %d 次", adapter.calls)
	}
}

// v19 分支：minted=TotalMintedReward，miner=minted−burn−explicit，service/burn 取真实计数器。
func TestGetRewardStreamTotalsV19(t *testing.T) {
	adapter := &fakeIndexAdapter{state: stateMap(t, rewardTotalsV19RealRow)}
	a := newTestAcl(&fakeIndexAgg{}, adapter, &fakeWinCountRepo{})

	totals, err := a.GetRewardStreamTotals(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if !totals.OK {
		t.Fatal("v19 取数成功应 OK=true")
	}
	if got, want := totals.Minted.String(), "110534789174400147324658484"; got != want {
		t.Fatalf("累计铸造量错: got %s want %s", got, want)
	}
	if got, want := totals.Miner.String(), "110533125171181697453530148"; got != want {
		t.Fatalf("累计矿工实收错: got %s want %s", got, want)
	}
	if got, want := totals.Service.String(), "1664003218449871124998"; got != want {
		t.Fatalf("累计服务流错: got %s want %s", got, want)
	}
	if got, want := totals.Burn.String(), "3338"; got != want {
		t.Fatalf("累计铸造即销毁错: got %s want %s", got, want)
	}
	// 不变式：minted = miner + service + burn。
	sum := totals.Miner.Add(totals.Service).Add(totals.Burn)
	if !sum.Equal(totals.Minted) {
		t.Fatalf("不变式被破坏: minted=%s != miner+service+burn=%s", totals.Minted, sum)
	}
}

// 取数失败：返回 err、OK=false、四者零值（调用方据此置 0，首页不 500）。
func TestGetRewardStreamTotalsError(t *testing.T) {
	adapter := &fakeIndexAdapter{err: errors.New("agg get actor f02 is empty")}
	a := newTestAcl(&fakeIndexAgg{}, adapter, &fakeWinCountRepo{})

	totals, err := a.GetRewardStreamTotals(context.Background(), 6429840)
	if err == nil {
		t.Fatal("取数失败必须返回 err")
	}
	if totals.OK {
		t.Fatal("取数失败必须 OK=false")
	}
	if !totals.Minted.IsZero() || !totals.Miner.IsZero() || !totals.Service.IsZero() || !totals.Burn.IsZero() {
		t.Fatalf("取数失败四者必须为零值: %+v", totals)
	}
}

// GetTotalRewards 语义不变：仍是矿工实收，且与 GetRewardStreamTotals 同源一次取数。
func TestGetTotalRewardsStillMinerMinted(t *testing.T) {
	adapter := &fakeIndexAdapter{state: stateMap(t, rewardTotalsV19RealRow)}
	a := newTestAcl(&fakeIndexAgg{}, adapter, &fakeWinCountRepo{})

	got, err := a.GetTotalRewards(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	if want := "110533125171181697453530148"; got.String() != want {
		t.Fatalf("total_rewards 仍应是矿工实收: got %s want %s", got, want)
	}
	if adapter.calls != 1 {
		t.Fatalf("应只读 f02 一次，实际 %d 次", adapter.calls)
	}
}
