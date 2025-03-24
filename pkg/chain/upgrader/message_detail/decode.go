package message_detail

import (
	"github.com/filecoin-project/go-state-types/actors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	v10 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v10"
	v11 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v11"
	v12 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v12"
	v13 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v13"
	v14 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v14"
	v15 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v15"
	v16 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v16"
	v8 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v8"
	v9 "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail/v9"
)

func ActorsVersionFromEpoch(epoch chain.Epoch) (actorVersion actors.Version, err error) {
	networkVersion := NetworkVersionFromEpoch(epoch)
	actorVersion, err = actors.VersionForNetwork(networkVersion)
	if err != nil {
		return
	}
	return
}

func DecodeParamsFromVersion(epoch chain.Epoch, params interface{}, methodName string) (ParamsResult interface{}, err error) {
	version, err := ActorsVersionFromEpoch(epoch)
	if err != nil {
		return
	}
	switch version {
	case 8:
		ParamsResult, err = v8.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 9:
		ParamsResult, err = v9.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 10:
		ParamsResult, err = v10.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 11:
		ParamsResult, err = v11.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 12:
		ParamsResult, err = v12.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 13:
		ParamsResult, err = v13.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 14:
		ParamsResult, err = v14.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 15:
		ParamsResult, err = v15.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	case 16:
		ParamsResult, err = v16.DecodeMessageParams(params, methodName)
		if err != nil {
			return
		}
	default:
		ParamsResult = params
	}
	return
}

func DecodeReturnsFromVersion(epoch chain.Epoch, returns interface{}, methodName string) (ReturnResult interface{}, err error) {
	version, err := ActorsVersionFromEpoch(epoch)
	if err != nil {
		return
	}
	switch version {
	case 8:
		ReturnResult, err = v8.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 9:
		ReturnResult, err = v9.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 10:
		ReturnResult, err = v10.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 11:
		ReturnResult, err = v11.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 12:
		ReturnResult, err = v12.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 13:
		ReturnResult, err = v13.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 14:
		ReturnResult, err = v14.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	case 15:
		ReturnResult, err = v15.DecodeMessageReturns(returns, methodName)
		if err != nil {
			return
		}
	default:
		ReturnResult = returns
	}
	return
}
