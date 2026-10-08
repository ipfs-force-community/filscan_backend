package browser

import (
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
)

// OK=true：累计四值原样透传给首页。
func TestRewardStreamTotalsIndicatorsOK(t *testing.T) {
	totals := acl.RewardStreamTotals{
		Minted:  decimal.RequireFromString("600"),
		Miner:   decimal.RequireFromString("400"),
		Service: decimal.RequireFromString("140"),
		Burn:    decimal.RequireFromString("60"),
		OK:      true,
	}
	minted, miner, service, burn := rewardStreamTotalsIndicators(totals)
	if !minted.Equal(totals.Minted) || !miner.Equal(totals.Miner) ||
		!service.Equal(totals.Service) || !burn.Equal(totals.Burn) {
		t.Fatalf("OK=true 应原样透传: %s %s %s %s", minted, miner, service, burn)
	}
}

// OK=false（f02 取数失败）：首页四者置 0，不 500、不 panic。
func TestRewardStreamTotalsIndicatorsNotOKZeroes(t *testing.T) {
	minted, miner, service, burn := rewardStreamTotalsIndicators(acl.RewardStreamTotals{OK: false})
	if !minted.IsZero() || !miner.IsZero() || !service.IsZero() || !burn.IsZero() {
		t.Fatalf("OK=false 必须四者置 0: %s %s %s %s", minted, miner, service, burn)
	}
}
