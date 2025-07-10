package message_detail

import (
	"sort"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

const (
	GENESIS    = 0
	BREEZE     = int64(buildconstants.UpgradeBreezeHeight)     //41280
	SMOKE      = int64(buildconstants.UpgradeSmokeHeight)      //51000
	IGNITION   = int64(buildconstants.UpgradeIgnitionHeight)   //94000
	REFUEL     = int64(buildconstants.UpgradeRefuelHeight)     //130800
	TAPE       = int64(buildconstants.UpgradeTapeHeight)       //140760
	KUMQUAT    = int64(buildconstants.UpgradeKumquatHeight)    //170000
	CALICO     = int64(buildconstants.UpgradeCalicoHeight)     //265200
	PERSIAN    = int64(buildconstants.UpgradePersianHeight)    //272400
	ORANGE     = int64(buildconstants.UpgradeOrangeHeight)     //336458
	TRUST      = int64(buildconstants.UpgradeTrustHeight)      //550321
	NORWEGIAN  = int64(buildconstants.UpgradeNorwegianHeight)  //665280
	TURBO      = int64(buildconstants.UpgradeTurboHeight)      //712320
	HYPERDRIVE = int64(buildconstants.UpgradeHyperdriveHeight) //892800
	CHOCOLATE  = int64(buildconstants.UpgradeChocolateHeight)  //1231620
	OHSNAP     = int64(buildconstants.UpgradeOhSnapHeight)     //1594680
	SKYR       = int64(buildconstants.UpgradeSkyrHeight)       //1960320
	SHARK      = int64(buildconstants.UpgradeSharkHeight)      //2383680
	HYGGE      = int64(buildconstants.UpgradeHyggeHeight)      //2683348
)

var (
	LIGHTNING               = chain.Epoch(build.UpgradeLightningHeight) //2809800
	THUNDER                 = chain.Epoch(build.UpgradeThunderHeight)   //2870280
	UpgradeWatermelonHeight = chain.Epoch(build.UpgradeWatermelonHeight)
	UpgradeDragonHeight     = chain.Epoch(build.UpgradeDragonHeight)
	UpgradeWaffleHeight     = chain.Epoch(build.UpgradeWaffleHeight)
	UpgradeTuktukHeight     = chain.Epoch(buildconstants.UpgradeTuktukHeight)
	UpgradeTeepHeight       = chain.Epoch(buildconstants.UpgradeTeepHeight)
	UpgradeTockHeight       = chain.Epoch(buildconstants.UpgradeTockHeight)
	VersionMap              = map[int64]network.Version{
		GENESIS:                         network.Version0,
		BREEZE:                          network.Version1,
		SMOKE:                           network.Version2,
		IGNITION:                        network.Version3,
		REFUEL:                          network.Version4,
		TAPE:                            network.Version5,
		KUMQUAT:                         network.Version6,
		CALICO:                          network.Version7,
		PERSIAN:                         network.Version8,
		ORANGE:                          network.Version9,
		TRUST:                           network.Version10,
		NORWEGIAN:                       network.Version11,
		TURBO:                           network.Version12,
		HYPERDRIVE:                      network.Version13,
		CHOCOLATE:                       network.Version14,
		OHSNAP:                          network.Version15,
		SKYR:                            network.Version16,
		SHARK:                           network.Version17,
		HYGGE:                           network.Version18,
		LIGHTNING.Int64():               network.Version19,
		THUNDER.Int64():                 network.Version20,
		UpgradeWatermelonHeight.Int64(): network.Version21,
		UpgradeDragonHeight.Int64():     network.Version22,
		UpgradeWaffleHeight.Int64():     network.Version23,
		UpgradeTuktukHeight.Int64():     network.Version24,
		UpgradeTeepHeight.Int64():       network.Version25,
		UpgradeTockHeight.Int64():       network.Version26,
	}
)

func NetworkVersionFromEpoch(epoch chain.Epoch) (targetVersion network.Version) {
	VersionList := []int64{
		GENESIS, BREEZE, SMOKE, IGNITION, REFUEL, TAPE, KUMQUAT, CALICO, PERSIAN, ORANGE, TRUST, NORWEGIAN, TURBO,
		HYPERDRIVE, CHOCOLATE, OHSNAP, SKYR, SHARK, HYGGE, LIGHTNING.Int64(), THUNDER.Int64(),
		UpgradeWatermelonHeight.Int64(), UpgradeDragonHeight.Int64(), UpgradeWaffleHeight.Int64(),
		UpgradeTuktukHeight.Int64(), UpgradeTeepHeight.Int64(), UpgradeTockHeight.Int64(),
	}

	index := sort.Search(len(VersionList), func(i int) bool { return VersionList[i] > epoch.Int64() }) - 1 // 使用二分查找算法查找区间索引
	if index < 0 {
		index = 0
	} else if index >= len(VersionList)-1 {
		index = len(VersionList) - 2
	}
	targetVersion = VersionMap[VersionList[index]]
	return
}
