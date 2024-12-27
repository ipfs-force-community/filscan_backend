package message

import "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"

var newMethodNames = map[MethodName]string{
	"Send(ethaccount)":  "InvokeEvm1",
	"Send(placeholder)": "InvokeEvm2",
}

type MethodName string

func (m MethodName) String() string {
	name, ok := newMethodNames[m]
	if ok {
		return name
	}
	return string(m)
}

func (m MethodName) CheckMethodName(toAddress chain.SmartAddress) (result string) {
	if m == "WithdrawBalance" {
		if toAddress.CrudeAddress() == "05" {
			result = "WithdrawBalance(market)"
		} else {
			result = "WithdrawBalance(miner)"
		}
	} else {
		result = string(m)
	}
	return
}
