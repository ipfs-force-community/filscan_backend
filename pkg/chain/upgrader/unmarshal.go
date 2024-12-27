package upgrader

import (
	"encoding/json"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/builtin/v9/power"
	"github.com/filecoin-project/go-state-types/builtin/v9/reward"
)

type PowerState struct {
	power.State
}

func UnmarshalerPowerState(data []byte) (state *PowerState, err error) {
	state = new(PowerState)
	err = json.Unmarshal(data, &state.State)
	if err != nil {
		return
	}
	return
}

type RewordState struct {
	reward.State
}

func UnmarshalerRewordState(data []byte) (state *RewordState, err error) {
	state = new(RewordState)
	err = json.Unmarshal(data, &state.State)
	if err != nil {
		return
	}
	return
}

type MarketState struct {
	market.State
}

func UnmarshalerMarketState(data []byte) (state *MarketState, err error) {
	state = new(MarketState)
	err = json.Unmarshal(data, &state.State)
	if err != nil {
		return
	}
	return
}

type MinerState struct {
	miner.State
}

func UnmarshalerMinerState(data []byte) (state *MinerState, err error) {
	state = new(MinerState)
	err = json.Unmarshal(data, &state.State)
	if err != nil {
		return
	}
	return
}
