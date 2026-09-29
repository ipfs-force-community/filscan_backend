package londobell

import (
	"math/big"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	sbig "github.com/filecoin-project/go-state-types/big"
	miner19 "github.com/filecoin-project/go-state-types/builtin/v19/miner"
)

// 32GiB 扇区，与 calibnet 上常见扇区大小一致。
const testSectorSize = uint64(34359738368)

const (
	tSimpleQAPower = uint64(1) // miner19.SIMPLE_QA_POWER
	tFullQAPower   = uint64(2) // miner19.FULL_QA_POWER
	tBothFlags     = uint64(3) // NV29 之后新扇区实测 flags = 0x3
)

func bint(v int64) *big.Int { return big.NewInt(v) }

// smul(f) = testSectorSize * f
func smul(f int64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(testSectorSize), big.NewInt(f))
}

// smulDiv(f, d) = testSectorSize * f / d（整除）
func smulDiv(f, d int64) *big.Int {
	return new(big.Int).Div(smul(f), big.NewInt(d))
}

type qaWant struct {
	full bool
	qa   *big.Int // nil = 不断言
	vdc  *big.Int
	dc   *big.Int
	cc   *big.Int
}

func TestQASplit(t *testing.T) {
	tests := []struct {
		name  string
		size  uint64
		flags uint64
		pbe   int64
		act   int64
		exp   int64
		dw    *big.Int
		vdw   *big.Int
		want  qaWant
	}{
		{
			// method 37 (UpgradeSectorQuality) 把老 CC 扇区升到 10x：VDW=0 但带 FULL 标志。
			name: "full_flag_vdw_zero_method37_upgraded_cc",
			size: testSectorSize, flags: tFullQAPower, pbe: 1000, act: 1000, exp: 2000000,
			dw: bint(0), vdw: bint(0),
			want: qaWant{true, smul(10), bint(0), bint(0), smul(10)},
		},
		{
			// NV29 之后新扇区：flags=0x3（SIMPLE|FULL），VDW 记的是全部 piece 时空。
			name: "full_flag_vdw_full_space_new_sector",
			size: testSectorSize, flags: tBothFlags, pbe: 1000, act: 1000, exp: 2000000,
			dw: bint(0), vdw: smul(1999000),
			want: qaWant{true, smul(10), bint(0), bint(0), smul(10)},
		},
		{
			// 非 full 标志、但 VDW 覆盖整个 QA 周期：NV29 之前靠 datacap 拿满 10x 的老扇区，
			// 迁移不回溯设标志，这里按满 QA 处理（与 lotus SectorIsFullQaPower 一致）。
			name: "vdw_covers_full_space_without_flag",
			size: testSectorSize, flags: tSimpleQAPower, pbe: 1000, act: 1000, exp: 2000000,
			dw: bint(0), vdw: smul(1999000),
			want: qaWant{true, smul(10), bint(0), bint(0), smul(10)},
		},
		{
			// 老 CC 扇区（无标志、无 deal）：1x，全部归容量算力。
			name: "legacy_cc_no_flag_vdw_zero",
			size: testSectorSize, flags: tSimpleQAPower, pbe: 1000, act: 1000, exp: 2000000,
			dw: bint(0), vdw: bint(0),
			want: qaWant{false, smul(1), bint(0), bint(0), smul(1)},
		},
		{
			// 非满 QA、VDW 占一半时空：qa = 5.5x，按权重切三桶（vdc=5x, dc=0, cc=0.5x）。
			name: "half_verified_split",
			size: testSectorSize, flags: tSimpleQAPower, pbe: 1000, act: 1000, exp: 2000000,
			dw: bint(0), vdw: smul(999500),
			want: qaWant{false, smulDiv(11, 2), smul(5), bint(0), smulDiv(1, 2)},
		},
		{
			// 续期扇区 PBE>Act：必须用 exp-pbe 作为 QA 周期。这里 exp-pbe=100、exp-act=1000，
			// 用对是 5.5x，用错（exp-act）会得到 1.45x。
			name: "renewed_pbe_gt_act_uses_exp_minus_pbe",
			size: testSectorSize, flags: uint64(0), pbe: 1000, act: 100, exp: 1100,
			dw: bint(0), vdw: smul(50),
			want: qaWant{false, smulDiv(11, 2), nil, nil, nil},
		},
		{
			// vdw+dw 超过 exp-act 周期：cc 权重 clamp 到 0，不出现负桶；余数（<2）留在 cc。
			name: "negative_cc_weight_clamped",
			size: testSectorSize, flags: tSimpleQAPower, pbe: 1000, act: 1000, exp: 2000,
			dw: smul(200), vdw: smul(900),
			want: qaWant{false, nil, nil, nil, nil},
		},
		{
			// 三桶权重全零（act==exp 且无 deal）：按约定 (0, 0, qa)。
			name: "all_weights_zero_falls_back_to_cc",
			size: testSectorSize, flags: tSimpleQAPower, pbe: 1000, act: 2000, exp: 2000,
			dw: bint(0), vdw: bint(0),
			want: qaWant{false, smul(1), bint(0), bint(0), smul(1)},
		},
		{
			// 异常数据 exp<=pbe：退回 exp-act（不 panic、不出现负 duration）。
			name: "exp_le_pbe_falls_back_to_activation",
			size: testSectorSize, flags: tSimpleQAPower, pbe: 3000, act: 1000, exp: 2000,
			dw: bint(0), vdw: bint(0),
			want: qaWant{false, smul(1), bint(0), bint(0), smul(1)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qa, vdc, dc, cc, full := QASplit(tt.size, tt.flags, tt.pbe, tt.act, tt.exp, tt.dw, tt.vdw)

			if qa == nil || vdc == nil || dc == nil || cc == nil {
				t.Fatalf("QASplit 返回了 nil：qa=%v vdc=%v dc=%v cc=%v", qa, vdc, dc, cc)
			}
			if full != tt.want.full {
				t.Errorf("full = %v, want %v", full, tt.want.full)
			}
			if tt.want.qa != nil && qa.Cmp(tt.want.qa) != 0 {
				t.Errorf("qa = %s, want %s", qa, tt.want.qa)
			}
			if tt.want.vdc != nil && vdc.Cmp(tt.want.vdc) != 0 {
				t.Errorf("vdc = %s, want %s", vdc, tt.want.vdc)
			}
			if tt.want.dc != nil && dc.Cmp(tt.want.dc) != 0 {
				t.Errorf("dc = %s, want %s", dc, tt.want.dc)
			}
			if tt.want.cc != nil && cc.Cmp(tt.want.cc) != 0 {
				t.Errorf("cc = %s, want %s", cc, tt.want.cc)
			}

			// 不变式：三桶非负，且精确求和等于 qa（上游按三桶之和与链上 QA 对账）。
			for name, v := range map[string]*big.Int{"vdc": vdc, "dc": dc, "cc": cc} {
				if v.Sign() < 0 {
					t.Errorf("%s = %s 为负", name, v)
				}
			}
			sum := new(big.Int).Add(vdc, dc)
			sum.Add(sum, cc)
			if sum.Cmp(qa) != 0 {
				t.Errorf("vdc+dc+cc = %s, want qa = %s", sum, qa)
			}
			if full && (vdc.Sign() != 0 || dc.Sign() != 0 || cc.Cmp(qa) != 0) {
				t.Errorf("full 扇区应 (0,0,qa)，实际 vdc=%s dc=%s cc=%s qa=%s", vdc, dc, cc, qa)
			}
		})
	}
}

// TestQASplitMatchesActorQAPowerForSector：qa 必须与 v19 actor 的权威实现逐位一致
// （非 full 分支即 QAPowerForWeight(size, exp-pbe, vdw)）。
func TestQASplitMatchesActorQAPowerForSector(t *testing.T) {
	cases := []struct {
		name  string
		flags uint64
		pbe   int64
		act   int64
		exp   int64
		dw    *big.Int
		vdw   *big.Int
	}{
		{"cc", tSimpleQAPower, 1000, 1000, 2000000, bint(0), bint(0)},
		{"partial_verified", tSimpleQAPower, 1000, 1000, 2000000, bint(0), smul(999500)},
		{"legacy_deal_weight_only", tSimpleQAPower, 1000, 1000, 2000000, smul(400000), bint(0)},
		{"renewed", 0, 1000, 100, 1100, bint(0), smul(50)},
		{"full_flag", tFullQAPower, 1000, 1000, 2000000, bint(0), bint(0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			qa, _, _, _, _ := QASplit(testSectorSize, c.flags, c.pbe, c.act, c.exp, c.dw, c.vdw)

			sector := &miner19.SectorOnChainInfo{
				Activation:         abi.ChainEpoch(c.act),
				Expiration:         abi.ChainEpoch(c.exp),
				DealWeight:         sbig.NewFromGo(c.dw),
				VerifiedDealWeight: sbig.NewFromGo(c.vdw),
				PowerBaseEpoch:     abi.ChainEpoch(c.pbe),
				Flags:              miner19.SectorOnChainInfoFlags(c.flags),
			}
			want := miner19.QAPowerForSector(abi.SectorSize(testSectorSize), sector).Int
			if qa.Cmp(want) != 0 {
				t.Errorf("qa = %s, actor QAPowerForSector = %s", qa, want)
			}
		})
	}
}

// TestQASplitRenewedSectorDuration：显式证明续期扇区用的是 exp-pbe，而不是 exp-act。
func TestQASplitRenewedSectorDuration(t *testing.T) {
	const (
		act = int64(100)
		pbe = int64(1000)
		exp = int64(1100)
	)
	vdw := smul(50) // VDW 覆盖 exp-pbe=100 的一半时空

	qa, _, _, _, full := QASplit(testSectorSize, 0, pbe, act, exp, bint(0), vdw)
	if full {
		t.Fatalf("不该判为 full")
	}
	want := smulDiv(11, 2) // 5.5x
	if qa.Cmp(want) != 0 {
		t.Errorf("qa = %s, want %s（用 exp-pbe=100 算）", qa, want)
	}

	// 用错 duration（exp-act=1000）会得到 1.45x，必须与上面不同。
	wrong := smulDiv(145, 100)
	if qa.Cmp(wrong) == 0 {
		t.Errorf("qa 与 exp-act 口径的结果 %s 相同，说明用错了 duration", wrong)
	}
}

// TestQASplitNegativeCCWeightClamped：cc 权重被 clamp 后只应剩舍入余数（<2）。
func TestQASplitNegativeCCWeightClamped(t *testing.T) {
	qa, vdc, dc, cc, full := QASplit(testSectorSize, tSimpleQAPower, 1000, 1000, 2000, smul(200), smul(900))
	if full {
		t.Fatalf("不该判为 full")
	}
	if cc.Sign() < 0 || cc.Cmp(bint(2)) >= 0 {
		t.Errorf("cc = %s，clamp 之后应只剩舍入余数（<2）", cc)
	}
	if vdc.Sign() <= 0 || dc.Sign() <= 0 {
		t.Errorf("vdc/dc 应为正：vdc=%s dc=%s", vdc, dc)
	}
	sum := new(big.Int).Add(new(big.Int).Add(vdc, dc), cc)
	if sum.Cmp(qa) != 0 {
		t.Errorf("三桶之和 %s != qa %s", sum, qa)
	}
}

func TestQASplitNilWeights(t *testing.T) {
	qa, vdc, dc, cc, full := QASplit(testSectorSize, 0, 1000, 1000, 2000000, nil, nil)
	if full {
		t.Fatalf("nil 权重不该判为 full")
	}
	if qa.Cmp(new(big.Int).SetUint64(testSectorSize)) != 0 {
		t.Errorf("qa = %s, want %d", qa, testSectorSize)
	}
	if vdc.Sign() != 0 || dc.Sign() != 0 || cc.Cmp(qa) != 0 {
		t.Errorf("nil 权重应 (0,0,qa)，实际 vdc=%s dc=%s cc=%s", vdc, dc, cc)
	}
}

func TestIsFullQaPower(t *testing.T) {
	size := testSectorSize
	if !IsFullQaPower(size, FlagFullQAPower, 1000, 2000000, bint(0)) {
		t.Error("FULL_QA_POWER 标志应判为 full")
	}
	if !IsFullQaPower(size, tBothFlags, 1000, 2000000, bint(0)) {
		t.Error("0x3 应判为 full")
	}
	if IsFullQaPower(size, FlagSimpleQAPower, 1000, 2000000, bint(0)) {
		t.Error("纯 CC 不应判为 full")
	}
	if !IsFullQaPower(size, FlagSimpleQAPower, 1000, 2000000, smul(1999000)) {
		t.Error("VDW 覆盖整周期（exp-pbe）应判为 full")
	}
	if IsFullQaPower(size, FlagSimpleQAPower, 1000, 2000000, smul(1000000)) {
		t.Error("VDW 只覆盖一半不应判为 full")
	}
	if IsFullQaPower(size, FlagSimpleQAPower, 3000, 2000, bint(0)) {
		t.Error("exp<=pbe 时不应判为 full")
	}
	if IsFullQaPower(size, FlagSimpleQAPower, 1000, 2000000, nil) {
		t.Error("nil VDW 不应判为 full")
	}
}
