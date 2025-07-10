package bo

import "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"

type NFTTransfer struct {
	po.NFTTransfer
	TokenUri string
	TokenUrl string
}
