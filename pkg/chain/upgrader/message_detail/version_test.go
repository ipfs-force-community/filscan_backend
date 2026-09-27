package message_detail

import (
	"testing"

	"github.com/filecoin-project/go-state-types/network"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// TestNetworkVersionFromEpoch 覆盖版本映射的两个边界行为：
//  1. 恰好落在某个升级高度上，应解析为该次升级对应的 network version；
//  2. 超出最后一个已排期高度时，应取最新版本（而不是被回退到倒数第二个）。
//
// 第 2 条是 NV29 修复的回归点：旧实现在 index 触达 len-1 时会强制回退到 len-2，
// 使得最新 NV 永远选不中。
//
// 注：不覆盖 calibnet genesis 早期（epoch < 60）—— calibnet 的 BREEZE~REFUEL 高度为负数，
// 使 VersionList 开头并非升序，二分查找在该区间本就不可靠。这是历史遗留问题，
// 只影响 calibnet 创世后一分钟内的区块，与本次升级无关。
func TestNetworkVersionFromEpoch(t *testing.T) {
	cases := []struct {
		name  string
		epoch int64
		want  network.Version
	}{
		{"hygge", HYGGE, network.Version18},
		{"lightning", LIGHTNING.Int64(), network.Version19},
		{"tock", UpgradeTockHeight.Int64(), network.Version26},
		{"goldenWeek", UpgradeGoldenWeekHeight.Int64(), network.Version27},
		{"fireHorse", UpgradeFireHorseHeight.Int64(), network.Version28},
		{"solstice", UpgradeSolsticeHeight.Int64(), network.Version29},
	}

	for _, c := range cases {
		if got := NetworkVersionFromEpoch(chain.Epoch(c.epoch)); got != c.want {
			t.Errorf("%s: epoch=%d got=Version%d want=Version%d", c.name, c.epoch, got, c.want)
		}
	}

	// 升级高度前一个 epoch 仍应停留在上一版
	before := []struct {
		name  string
		epoch int64
		want  network.Version
	}{
		{"fireHorse-1", UpgradeFireHorseHeight.Int64() - 1, network.Version27},
		{"solstice-1", UpgradeSolsticeHeight.Int64() - 1, network.Version28},
	}
	for _, c := range before {
		if got := NetworkVersionFromEpoch(chain.Epoch(c.epoch)); got != c.want {
			t.Errorf("%s: epoch=%d got=Version%d want=Version%d", c.name, c.epoch, got, c.want)
		}
	}

	// 远超最后一个已排期高度时取最新版本
	if got := NetworkVersionFromEpoch(chain.Epoch(UpgradeSolsticeHeight.Int64() + 1)); got != network.Version29 {
		t.Errorf("beyond solstice: got=Version%d want=Version29", got)
	}
}

// TestActorsVersionFromEpochNV29 确认 NV29 能正确落到 actors v19。
func TestActorsVersionFromEpochNV29(t *testing.T) {
	av, err := ActorsVersionFromEpoch(chain.Epoch(UpgradeSolsticeHeight.Int64()))
	if err != nil {
		t.Fatalf("ActorsVersionFromEpoch: %v", err)
	}
	if av != 19 {
		t.Errorf("actors version = %d, want 19", av)
	}

	av, err = ActorsVersionFromEpoch(chain.Epoch(UpgradeFireHorseHeight.Int64()))
	if err != nil {
		t.Fatalf("ActorsVersionFromEpoch: %v", err)
	}
	if av != 18 {
		t.Errorf("actors version at FireHorse = %d, want 18", av)
	}
}
