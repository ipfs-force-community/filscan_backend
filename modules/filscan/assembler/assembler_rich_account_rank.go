package assembler

import (
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
)

type RichAccountRankAssembler struct {
}

func (r RichAccountRankAssembler) ToRichAccountRank(richAccount *bo.RichAccountRank, totalAccountBalance decimal.Decimal, lastRxTime int64) (richAccountFil *filscan.RichAccount, err error) {
	richAccountFil = &filscan.RichAccount{
		AccountID:          richAccount.Actor,
		AccountAddress:     richAccount.Actor,
		AccountType:        richAccount.Type,
		Balance:            richAccount.Balance,
		BalancePercentage:  decimal.Decimal{},
		LatestTransferTime: lastRxTime,
	}
	if !totalAccountBalance.IsZero() {
		richAccountFil.BalancePercentage = richAccountFil.Balance.Div(totalAccountBalance)
	}
	if richAccount.Type == "storageminer" {
		richAccountFil.AccountType = "miner"
	}

	return
}
