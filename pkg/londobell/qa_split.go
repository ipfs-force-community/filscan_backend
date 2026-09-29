package londobell

import (
	"math/big"

	"github.com/filecoin-project/go-state-types/abi"
	sbig "github.com/filecoin-project/go-state-types/big"
	miner19 "github.com/filecoin-project/go-state-types/builtin/v19/miner"
)

// 本文件是 NV29（Solstice / FIP-0118，calibnet 高度 4109133）之后的 miner 扇区算力口径实现。
//
// ⚠️ 同源实现：londobell 仓库里有一份逐行等价的拷贝
// londobell/cmd/londobell-api/controller/adapter/qa_split.go。
// 两个仓库各自持有 lotus / go-state-types 依赖，无法共享代码，任何改动必须两边同步。
//
// 权威口径（go-state-types@v0.19.x builtin/v19/miner/policy.go:200-208）：
//
//	QAPowerForSector(size, sector) =
//	    sector.Flags&FULL_QA_POWER != 0 ? QAPowerMax(size) = 10*size
//	                                    : QAPowerForWeight(size, Expiration-PowerBaseEpoch, VerifiedDealWeight)
//
// 与 v11 老口径的两点差别：
//  1. 新增 FULL_QA_POWER 标志（FIP-0118，v19/miner/miner_state.go:161-168）：带标志的扇区
//     不论 deal 内容一律 10x —— 包括新扇区（创建即带标志）和 method 37 UpgradeSectorQuality
//     事后升级的老扇区；
//  2. QA 周期起点从 Activation 改成 PowerBaseEpoch（v19/miner/miner_state.go:190）。续期过的
//     扇区 PowerBaseEpoch > Activation，继续用 Expiration-Activation 会把 duration 算大、
//     qa 算小。
//
// DealWeight 在 v19 的 QualityForWeight 里已被移除（不再影响 QA，只剩 DC 归属的意义）；
// FIP-0118 之后新扇区的 DealWeight 恒为 0，piece 的时空全部记在 VerifiedDealWeight 里。
const (
	// FlagSimpleQAPower = 1<<0：FIP-0045 的 QA 机制标志，v19 的 miner actor 已不再读取。
	FlagSimpleQAPower = uint64(miner19.SIMPLE_QA_POWER)
	// FlagFullQAPower = 1<<1：FIP-0118 引入，带此标志的扇区恒为最大 QA 算力（10x）。
	FlagFullQAPower = uint64(miner19.FULL_QA_POWER)
	// QAMaxMultiplier = VerifiedDealWeightMultiplier / QualityBaseMultiplier = 10。
	QAMaxMultiplier = int64(10)
)

// IsFullQaPower 判定扇区是否等价于「满 QA 算力（10x）」：
//   - 带 FULL_QA_POWER 标志（NV29 之后新扇区，或 method 37 升级过的老扇区）；
//   - 或者 VerifiedDealWeight 覆盖了整个 QA 周期（NV29 之前靠 datacap 拿满 10x 的老扇区：
//     迁移不回溯设标志，见 lotus chain/actors/builtin/miner/utils.go SectorIsFullQaPower）。
//
// duration 用 Expiration-PowerBaseEpoch，与 v19 的 QA 周期一致。
func IsFullQaPower(size uint64, flags uint64, pbe, exp int64, vdw *big.Int) bool {
	if flags&FlagFullQAPower != 0 {
		return true
	}
	dur := exp - pbe
	if dur <= 0 {
		return false
	}
	if vdw == nil {
		return false
	}
	return vdw.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(size), big.NewInt(dur))) >= 0
}

// QASplit 把一个扇区按 NV29 口径切成 QA 总量与 VDC / DC / CC 三个归属桶。
//
// 返回的三桶之和恒等于 qa（余数归 cc），单位为 QA 算力（byte）；full 表示该扇区走上了
// 「满 QA」分支（此时 vdc=dc=0、cc=qa：FIP-0118 之后没有 verified deal 概念，整块算力
// 全部归容量算力）。
//
// 参数：size 扇区大小；flags 链上 SectorOnChainInfo.Flags；pbe/act/exp 为
// PowerBaseEpoch / Activation / Expiration；dw/vdw 为 DealWeight / VerifiedDealWeight（可为 nil）。
func QASplit(size uint64, flags uint64, pbe, act, exp int64, dw, vdw *big.Int) (qa, vdc, dc, cc *big.Int, full bool) {
	dw = nonNilInt(dw)
	vdw = nonNilInt(vdw)
	sizeInt := new(big.Int).SetUint64(size)

	// QA 周期：FIP-0118 之后起点是 PowerBaseEpoch。数据异常（exp<=pbe，多为老数据未带该字段）
	// 时退回 Activation；两者都非正时下面 actor 的质量公式会除零，直接返回零桶。
	dur := exp - pbe
	if dur <= 0 {
		dur = exp - act
	}
	if dur <= 0 {
		return new(big.Int), new(big.Int), new(big.Int), new(big.Int), flags&FlagFullQAPower != 0
	}
	// 实际生效的 QA 周期起点：正常取 PowerBaseEpoch，退回时取 Activation。
	pbeEff := exp - dur

	durAct := exp - act
	if durAct < 0 {
		durAct = 0
	}

	full = IsFullQaPower(size, flags, pbeEff, exp, vdw)

	if flags&FlagFullQAPower != 0 {
		// 显式走 actor 的 QAPowerMax：10*size。
		qa = new(big.Int).Mul(sizeInt, big.NewInt(QAMaxMultiplier))
	} else {
		// 其余情况（含「没带标志、仅靠 VDW 覆盖整周期」的满 QA 老扇区）用 actor 的权威实现，
		// 避免在这里再抄一遍质量公式。
		sector := &miner19.SectorOnChainInfo{
			Activation:         abi.ChainEpoch(act),
			Expiration:         abi.ChainEpoch(exp),
			DealWeight:         sbig.NewFromGo(dw),
			VerifiedDealWeight: sbig.NewFromGo(vdw),
			PowerBaseEpoch:     abi.ChainEpoch(pbeEff),
			Flags:              miner19.SectorOnChainInfoFlags(flags),
		}
		qa = miner19.QAPowerForSector(abi.SectorSize(size), sector).Int
	}

	if full {
		// FIP-0118：满 QA 扇区没有 verified deal 概念，整块算力归容量算力。
		return qa, new(big.Int), new(big.Int), new(big.Int).Set(qa), true
	}

	// 非满 QA：先算三个原始权重，再按占比把 qa 切到三桶。
	vdcW := new(big.Int).Mul(vdw, big.NewInt(QAMaxMultiplier))
	dcW := new(big.Int).Set(dw)
	raw := new(big.Int).Mul(sizeInt, big.NewInt(durAct))
	ccW := new(big.Int).Sub(raw, vdw)
	ccW.Sub(ccW, dw)
	if ccW.Sign() < 0 {
		// vdw+dw 超过 exp-act 周期（续期扇区/边界数据）时 clamp 到 0，不让空桶为负。
		ccW = new(big.Int)
	}

	all := new(big.Int).Add(vdcW, dcW)
	all.Add(all, ccW)
	if all.Sign() == 0 {
		// 三个权重全零（例如 act==exp 且无 deal）：全部归容量算力。
		return qa, new(big.Int), new(big.Int), new(big.Int).Set(qa), false
	}

	vdc = new(big.Int).Div(new(big.Int).Mul(qa, vdcW), all)
	dc = new(big.Int).Div(new(big.Int).Mul(qa, dcW), all)
	// 余数留给 cc：保证 vdc+dc+cc 恒等于 qa（上游按三桶求和跟链上 QA 对账）。
	cc = new(big.Int).Sub(qa, vdc)
	cc.Sub(cc, dc)
	return qa, vdc, dc, cc, false
}

func nonNilInt(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return v
}
