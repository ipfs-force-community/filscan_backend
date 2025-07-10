package fns_task

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/events"
)

func hexToBytes(s string) []byte {
	a, _ := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	return a
}

func NewFnsTask(fevmRepo repository.FEvmRepo, fnsSaver repository.FnsSaver) *FnsTask {
	t := &FnsTask{
		fevm:      fevmRepo,
		acl:       fnsSaver,
		contracts: []fns.Contract{fns.NewOpengateToken(), fns.NewFilfoxToken()},
		accept:    map[string]struct{}{},
	}
	
	for _, v := range t.contracts {
		t.accept[strings.ToLower(v.RegistrarContract())] = struct{}{}
		t.accept[strings.ToLower(v.FNSRegistryContract())] = struct{}{}
		t.accept[strings.ToLower(v.PublicResolverContract())] = struct{}{}
		t.accept[strings.ToLower(v.ReverseRegistrarContract())] = struct{}{}
	}
	
	return t
}

var _ syncer.Task = (*FnsTask)(nil)

type FnsTask struct {
	acl       repository.FnsSaver
	fevm      repository.FEvmRepo
	contracts []fns.Contract
	accept    map[string]struct{}
}

func (o FnsTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	err = o.acl.DeleteEventsAfterEpoch(ctx, safeClearEpoch)
	if err != nil {
		return
	}
	return
}

func (o FnsTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = o.acl.DeleteEventsAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (o FnsTask) Name() string {
	return "fns-task"
}

func (o FnsTask) save(ctx *syncer.Context, items []*po.FNSEvent) (err error) {
	err = o.acl.AddEvents(ctx.Context(), items)
	if err != nil {
		return
	}
	return
}

func (o FnsTask) Exec(ctx *syncer.Context) (err error) {
	
	if ctx.Empty() {
		return
	}
	
	r, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := r.([]*londobell.TraceMessage)
	
	n := events.NewEvents(ctx.Agg(), traces)
	
	es, err := n.GetEvents(ctx.Context())
	if err != nil {
		err = fmt.Errorf("parse events error: %s", err)
		return
	}
	
	var acceptEvents []*po.FNSEvent
	for _, v := range es {
		var item *po.FNSEvent
		item, err = o.acceptEvent(ctx.Epoch(), v)
		if err != nil {
			return
		}
		if item != nil {
			acceptEvents = append(acceptEvents, item)
		}
	}
	
	// 保存接收的事件
	err = o.save(ctx, acceptEvents)
	if err != nil {
		return
	}
	
	return
}

func (o FnsTask) acceptEvent(epoch chain.Epoch, event *events.Event) (item *po.FNSEvent, err error) {
	
	ethAddr, err := ethtypes.CastEthAddress(hexToBytes(event.Address))
	if err != nil {
		err = fmt.Errorf("%s cast to eth address error: %s", event.Address, err)
		return
	}
	
	if _, ok := o.accept[strings.ToLower(ethAddr.String())]; !ok {
		return
	}
	
	eventName := ""
	{
		var en string
		en, err = o.fevm.GetEventNameBySignature(context.Background(), event.Topics[0])
		if err != nil {
			return
		}
		if en != "" {
			eventName = en
		}
	}
	
	methodName := ""
	{
		if len(event.TX.Input) > 10 {
			methodName = event.TX.Input[0:10]
			var mn string
			mn, err = o.fevm.GetMethodNameBySignature(context.Background(), methodName)
			if err != nil {
				return
			}
			if mn != "" {
				methodName = mn
			}
		}
	}
	
	logIndex := new(big.Int)
	logIndex.SetString(strings.TrimPrefix(event.LogIndex, "0x"), 16)
	
	item = &po.FNSEvent{
		Epoch:      epoch.Int64(),
		Cid:        event.XCid,
		LogIndex:   logIndex.Int64(),
		Contract:   ethAddr.String(),
		EventName:  eventName,
		Topics:     event.Topics,
		Data:       event.Data,
		Removed:    event.Removed,
		MethodName: methodName,
	}
	
	return
}
