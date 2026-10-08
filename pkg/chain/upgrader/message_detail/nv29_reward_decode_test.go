package message_detail

import (
	"bytes"
	"encoding/base64"
	"io"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	gtbig "github.com/filecoin-project/go-state-types/big"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	v19 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v19"
)

// NV29(Solstice / FIP-0118) 奖励流方法解码测试。
//
// 参数/返回用 go-state-types v0.19.0 builtin/v19/reward 的**真实类型经 MarshalCBOR 生成的字节**
// （与链上 params 同一编码器，非手搓），地址/流 ID/金额取 2026-10-08 Calibnet 实测值
// （impact/README §0）：隐式流 ID=1、服务流 ID=2、份额表 f014260492 73.7463% / f016424204 26.2537%、
// Writer f011836172、TStart 4135054、当期 f014260492 已 Claim 10,378.06 FIL。

const (
	caliServiceStreamID = 2
	caliServiceWeight   = 450000000000000000        // 45% (Denom=1e18)
	caliShareA          = 737463126843657817        // f014260492 73.7463%
	caliShareB          = 262536873156342183        // f016424204 26.2537%
	caliClaimAtto       = "10378060000000000000000" // 10,378.06 FIL
)

func mustAddr(t *testing.T, s string) address.Address {
	t.Helper()
	a, err := address.NewFromString(s)
	if err != nil {
		t.Fatalf("构造地址 %s 失败: %s", s, err)
	}
	return a
}

func mustBig(t *testing.T, s string) gtbig.Int {
	t.Helper()
	i, err := gtbig.FromString(s)
	if err != nil {
		t.Fatalf("构造整数 %s 失败: %s", s, err)
	}
	return i
}

// cborParams 把任意带 MarshalCBOR 的 go-state-types 值编成 DecodeMessageParams 期望的输入形状。
func cborParams(t *testing.T, m interface{ MarshalCBOR(w io.Writer) error }) map[string]interface{} {
	t.Helper()
	var buf bytes.Buffer
	if err := m.MarshalCBOR(&buf); err != nil {
		t.Fatalf("MarshalCBOR 失败: %s", err)
	}
	return map[string]interface{}{
		"$binary": map[string]interface{}{
			"base64": base64.StdEncoding.EncodeToString(buf.Bytes()),
		},
	}
}

// ClaimExported 参数（流 ID + 钱包列表）在 NV29 高度按 v19 解码。
func TestDecodeClaimExportedParams(t *testing.T) {
	params := &reward19.ClaimParams{
		ID:      reward19.StreamID(caliServiceStreamID),
		Wallets: []address.Address{mustAddr(t, "f014260492")},
	}

	res, err := DecodeParamsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), cborParams(t, params), "ClaimExported")
	if err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	got, ok := res.(*v19.ClaimParams)
	if !ok {
		t.Fatalf("结果类型应为 *v19.ClaimParams，实际 %T", res)
	}
	if got.ID != caliServiceStreamID {
		t.Fatalf("流 ID 错: got %d want %d", got.ID, caliServiceStreamID)
	}
	if len(got.Wallets) != 1 || got.Wallets[0] != "f014260492" {
		t.Fatalf("钱包列表错: got %v want [f014260492]", got.Wallets)
	}
}

// ClaimExported 返回（各钱包金额）在 NV29 高度按 v19 解码。
func TestDecodeClaimExportedReturn(t *testing.T) {
	ret := &reward19.ClaimReturn{
		Amounts: []abi.TokenAmount{mustBig(t, caliClaimAtto)},
	}

	res, err := DecodeReturnsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), cborParams(t, ret), "ClaimExported")
	if err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	got, ok := res.(*v19.ClaimReturn)
	if !ok {
		t.Fatalf("结果类型应为 *v19.ClaimReturn，实际 %T", res)
	}
	if len(got.Amounts) != 1 || got.Amounts[0] != caliClaimAtto {
		t.Fatalf("金额错: got %v want [%s]", got.Amounts, caliClaimAtto)
	}
}

// RegisterStreamExported 参数（流 ID + 权重 + 分配 + 激活高度）按 v19 解码（含真实份额表）。
func TestDecodeRegisterStreamExportedParams(t *testing.T) {
	params := &reward19.RegisterStreamParams{
		ID: reward19.StreamID(caliServiceStreamID),
		Weight: reward19.WeightRecord{
			VStart: caliServiceWeight,
			Slope:  0,
			TStart: abi.ChainEpoch(4135054),
			Floor:  caliServiceWeight,
			Cap:    caliServiceWeight,
		},
		Distribution: &reward19.DistributionInit{
			Writer: mustAddr(t, "f011836172"),
			Shares: []reward19.RecipientShare{
				{Recipient: mustAddr(t, "f014260492"), Share: caliShareA},
				{Recipient: mustAddr(t, "f016424204"), Share: caliShareB},
			},
		},
		ActivationEpoch: abi.ChainEpoch(4135054),
	}

	res, err := DecodeParamsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), cborParams(t, params), "RegisterStreamExported")
	if err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	got, ok := res.(*v19.RegisterStreamParams)
	if !ok {
		t.Fatalf("结果类型应为 *v19.RegisterStreamParams，实际 %T", res)
	}
	if got.ID != caliServiceStreamID || got.ActivationEpoch != 4135054 {
		t.Fatalf("ID/激活高度错: %+v", got)
	}
	if got.Weight.VStart != caliServiceWeight || got.Weight.Floor != caliServiceWeight || got.Weight.Cap != caliServiceWeight {
		t.Fatalf("权重记录错: %+v", got.Weight)
	}
	if got.Distribution == nil || got.Distribution.Writer != "f011836172" {
		t.Fatalf("分配 Writer 错: %+v", got.Distribution)
	}
	if len(got.Distribution.Shares) != 2 ||
		got.Distribution.Shares[0].Recipient != "f014260492" || got.Distribution.Shares[0].Share != caliShareA ||
		got.Distribution.Shares[1].Recipient != "f016424204" || got.Distribution.Shares[1].Share != caliShareB {
		t.Fatalf("份额表错: %+v", got.Distribution.Shares)
	}
}

// SetSharesExported（份额表替换）与 CancelPendingExported（nil ID = 日程级）按 v19 解码。
func TestDecodeSetSharesAndCancelPendingParams(t *testing.T) {
	shares := &reward19.SetSharesParams{
		ID: reward19.StreamID(caliServiceStreamID),
		Shares: []reward19.RecipientShare{
			{Recipient: mustAddr(t, "f014260492"), Share: caliShareA},
		},
	}
	res, err := DecodeParamsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), cborParams(t, shares), "SetSharesExported")
	if err != nil {
		t.Fatalf("SetShares 解码失败: %s", err)
	}
	gotShares, ok := res.(*v19.SetSharesParams)
	if !ok || gotShares.ID != caliServiceStreamID || len(gotShares.Shares) != 1 || gotShares.Shares[0].Share != caliShareA {
		t.Fatalf("SetShares 结果错: %T %+v", res, res)
	}

	cancel := &reward19.CancelPendingParams{ID: nil, Op: reward19.PendingWriteOpSetWeightRecords}
	res, err = DecodeParamsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), cborParams(t, cancel), "CancelPendingExported")
	if err != nil {
		t.Fatalf("CancelPending 解码失败: %s", err)
	}
	gotCancel, ok := res.(*v19.CancelPendingParams)
	if !ok || gotCancel.ID != nil || gotCancel.Op != uint8(reward19.PendingWriteOpSetWeightRecords) {
		t.Fatalf("CancelPending 结果错: %T %+v", res, res)
	}
}

// 其余 8 个奖励流方法的返回都是空值，按 v19 解码为 *abi.EmptyValue（不崩、不 nil）。
func TestDecodeNV29RewardStreamEmptyReturns(t *testing.T) {
	// 空值返回在链上就是零字节；EmptyValue.MarshalCBOR 只在 nil 指针下写零字节（非 nil 会报错）。
	empty := (*abi.EmptyValue)(nil)
	for _, name := range []string{
		"SetWeightRecordsExported", "StepWeightRecordsExported", "RegisterStreamExported",
		"RemoveStreamExported", "SetDistributionExported", "SetSharesExported",
		"ReplaceAddressExported", "CancelPendingExported",
	} {
		res, err := DecodeReturnsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), cborParams(t, empty), name)
		if err != nil {
			t.Fatalf("%s 返回解码失败: %s", name, err)
		}
		if _, ok := res.(*abi.EmptyValue); !ok {
			t.Fatalf("%s 返回应为 *abi.EmptyValue，实际 %T", name, res)
		}
	}
}

// 未知方法保持 nil,nil 的降级行为（不 panic、不报错）。
func TestDecodeUnknownMethodDegrades(t *testing.T) {
	params := &reward19.ClaimParams{ID: 1}
	raw := cborParams(t, params)
	res, err := DecodeParamsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), raw, "SomeUnknownNV29Method")
	if err != nil || res != nil {
		t.Fatalf("未知方法参数应 (nil,nil)，得到 res=%v err=%v", res, err)
	}
	res, err = DecodeReturnsFromVersion(chain.Epoch(UpgradeSolsticeHeight.Int64()), raw, "SomeUnknownNV29Method")
	if err != nil || res != nil {
		t.Fatalf("未知方法返回应 (nil,nil)，得到 res=%v err=%v", res, err)
	}
}
