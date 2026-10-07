package browser

import (
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
)

// OK=true：四个金额原样透传给首页。
func TestRewardStreamIndicators24HOK(t *testing.T) {
	d := acl.RewardStreamDeltas24H{
		Miner:   decimal.RequireFromString("400"),
		Service: decimal.RequireFromString("140"),
		Burn:    decimal.RequireFromString("60"),
		Total:   decimal.RequireFromString("600"),
		OK:      true,
	}
	miner, service, burn, total := rewardStreamIndicators24H(d)
	if !miner.Equal(d.Miner) || !service.Equal(d.Service) || !burn.Equal(d.Burn) || !total.Equal(d.Total) {
		t.Fatalf("OK=true 应原样透传: %s %s %s %s", miner, service, burn, total)
	}
}

// OK=false（快照不足/取数失败）：首页四者置 0，不 500、不 panic。
func TestRewardStreamIndicators24HNotOKZeroes(t *testing.T) {
	miner, service, burn, total := rewardStreamIndicators24H(acl.RewardStreamDeltas24H{OK: false})
	if !miner.IsZero() || !service.IsZero() || !burn.IsZero() || !total.IsZero() {
		t.Fatalf("OK=false 必须四者置 0: %s %s %s %s", miner, service, burn, total)
	}
}
