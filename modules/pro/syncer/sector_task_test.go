package prosyncer

import (
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 32GiB
const testSectorSize = int64(34359738368)

func decSectors(f int64) decimal.Decimal {
	return decimal.NewFromInt(testSectorSize * f)
}

// TestPrepareSectorQACaliber 覆盖 prepareSector 的 NV29 高度分流：
//   - epoch >= UpgradeSolsticeHeight（nv29=true）走 pkg/londobell.QASplit 的 v19 口径；
//   - 之前（nv29=false）保持 v11 老口径，历史数据不重算。
func TestPrepareSectorQACaliber(t *testing.T) {
	s := SectorTask{}
	size := decimal.NewFromInt(testSectorSize)

	tests := []struct {
		name  string
		nv29  bool
		sect  londobell.MinerSector
		wantV decimal.Decimal
		wantD decimal.Decimal
		wantC decimal.Decimal
	}{
		{
			// NV29 之后新扇区：flags=0x3（SIMPLE|FULL），VDW 记的是全部 piece 时空 → 10x 全归 CC。
			name: "nv29_new_sector_full_flag",
			nv29: true,
			sect: londobell.MinerSector{
				Activation: 1000, PowerBaseEpoch: 1000, Expiration: 2000000,
				Flags: 3, FullQaPower: true,
				DealWeight:         decimal.Zero,
				VerifiedDealWeight: decSectors(1999000),
			},
			wantV: decimal.Zero,
			wantD: decimal.Zero,
			wantC: decSectors(10),
		},
		{
			// method 37（UpgradeSectorQuality）把老 CC 扇区升到 10x：VDW=0、带 FULL 标志 → 必须 10x。
			name: "nv29_method37_upgraded_old_cc",
			nv29: true,
			sect: londobell.MinerSector{
				Activation: 1000, PowerBaseEpoch: 1000, Expiration: 2000000,
				Flags: 2, FullQaPower: true,
				DealWeight:         decimal.Zero,
				VerifiedDealWeight: decimal.Zero,
			},
			wantV: decimal.Zero,
			wantD: decimal.Zero,
			wantC: decSectors(10),
		},
		{
			// 续期扇区 PBE>Act：QA 周期用 exp-pbe=100（不是 exp-act=1000），qa = 5.5x。
			// 桶权重：vdcW=10*vdw=500S、dcW=0、ccW=size*(exp-act)-vdw=950S，all=1450S，
			// 因此 vdc = 5.5S*500/1450 = 65165021042、cc = qa-vdc = 123813539982。
			// （用错 duration 会是另一组数：这也正是本用例的要害。）
			name: "nv29_renewed_sector_uses_power_base_epoch",
			nv29: true,
			sect: londobell.MinerSector{
				Activation: 100, PowerBaseEpoch: 1000, Expiration: 1100,
				Flags: 0,
				DealWeight:         decimal.Zero,
				VerifiedDealWeight: decSectors(50),
			},
			wantV: decimal.NewFromInt(65165021042),
			wantD: decimal.Zero,
			wantC: decimal.NewFromInt(123813539982),
		},
		{
			// 老 CC 扇区（无标志、无 deal）：1x、全归 CC。
			name: "nv29_legacy_cc",
			nv29: true,
			sect: londobell.MinerSector{
				Activation: 1000, PowerBaseEpoch: 1000, Expiration: 2000000,
				Flags: 1,
				DealWeight:         decimal.Zero,
				VerifiedDealWeight: decimal.Zero,
			},
			wantV: decimal.Zero,
			wantD: decimal.Zero,
			wantC: decSectors(1),
		},
		{
			// 历史（epoch < Solstice）保持 v11 老口径：同样输入按占比切 qa，
			// VDW 覆盖整周期时全部落在 VDC —— 与上面的 NV29 结果正好相反，证明分流生效。
			name: "pre_solstice_keeps_v11_caliber",
			nv29: false,
			sect: londobell.MinerSector{
				Activation: 1000, PowerBaseEpoch: 1000, Expiration: 2000000,
				Flags: 3, FullQaPower: true,
				DealWeight:         decimal.Zero,
				VerifiedDealWeight: decSectors(1999000),
			},
			wantV: decSectors(10),
			wantD: decimal.Zero,
			wantC: decimal.Zero,
		},
		{
			// 历史老 CC：1x、全归 CC。
			name: "pre_solstice_legacy_cc",
			nv29: false,
			sect: londobell.MinerSector{
				Activation: 1000, PowerBaseEpoch: 1000, Expiration: 2000000,
				Flags: 1,
				DealWeight:         decimal.Zero,
				VerifiedDealWeight: decimal.Zero,
			},
			wantV: decimal.Zero,
			wantD: decimal.Zero,
			wantC: decSectors(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := s.prepareSector(120, &tt.sect, size, tt.nv29)
			if !item.Vdc.Equal(tt.wantV) {
				t.Errorf("Vdc = %s, want %s", item.Vdc, tt.wantV)
			}
			if !item.Dc.Equal(tt.wantD) {
				t.Errorf("Dc = %s, want %s", item.Dc, tt.wantD)
			}
			if !item.Cc.Equal(tt.wantC) {
				t.Errorf("Cc = %s, want %s", item.Cc, tt.wantC)
			}
		})
	}
}

// TestPowerNearEqual：1ppm 相对容差。
func TestPowerNearEqual(t *testing.T) {
	big15 := decimal.NewFromInt(1_000_000_000_000_000) // 1e15，量级接近 1 PiB

	tests := []struct {
		name string
		a, b decimal.Decimal
		want bool
	}{
		{"完全相等", big15, big15, true},
		{"0.1% 之差（超容差）", big15, big15.Mul(decimal.NewFromFloat(1.001)), false},
		{"1e9 绝对差 = 1ppm 之内", big15, big15.Add(decimal.NewFromInt(999_999_999)), true},
		{"2e6 绝对差 = 2ppb 之内", big15, big15.Add(decimal.NewFromInt(2_000_000)), true},
		{"2 倍之差", big15, big15.Mul(decimal.NewFromInt(2)), false},
		{"一边为零", decimal.NewFromInt(1000), decimal.Zero, false},
		{"都为零", decimal.Zero, decimal.Zero, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := powerNearEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("powerNearEqual(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
