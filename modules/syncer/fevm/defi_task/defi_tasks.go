package defi_task

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/bifrost"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/collectIf"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/defi_protocols"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/filFI"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/filetFi"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/filliquid"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/glif"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/hashking"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/hashmix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/mFIlProtocol"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/mineFi"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/sft"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/stFil"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/themis_pro"
)

var subTask []defi_protocols.DefiProtocols

func init() {
	subTask = append(subTask, stFil.StakeFil{})
	subTask = append(subTask, bifrost.BifrostLiquidStaking{})
	subTask = append(subTask, collectIf.Collectif{})
	subTask = append(subTask, filetFi.FiletFi{})
	subTask = append(subTask, filFI.FilFi{})
	subTask = append(subTask, glif.Glif{})
	subTask = append(subTask, hashking.HashKing{})
	subTask = append(subTask, hashmix.HashMix{})
	subTask = append(subTask, mFIlProtocol.MFIL{})
	subTask = append(subTask, mineFi.MineFi{})
	subTask = append(subTask, sft.SFT{})
	subTask = append(subTask, themis_pro.ThemisPro{})
	subTask = append(subTask, filliquid.FILLiquid{})
}
