package events

import (
	"context"
	"fmt"
	"github.com/gozelle/async/parallel"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"strings"
	"sync"
)

type Event struct {
	XCid string
	*londobell.EthReceiptLog
	TX      *londobell.EthTransaction
	Receipt *londobell.EthReceipt
}

func NewEvents(agg londobell.Agg, traces []*londobell.TraceMessage) *Events {
	return &Events{agg: agg, traces: traces}
}

type Events struct {
	agg    londobell.Agg
	traces []*londobell.TraceMessage
	events []*Event
	parsed bool
	lock   sync.Mutex
}

func (p *Events) GetEvents(ctx context.Context) (events []*Event, err error) {
	
	p.lock.Lock()
	defer func() {
		p.parsed = true
		p.lock.Unlock()
	}()
	
	if p.parsed {
		events = p.events
		return
	}
	
	var messages []*londobell.TraceMessage
	for _, v := range p.traces {
		if v.Depth == 1 && v.Detail != nil && strings.Contains(v.Detail.Actor, "evm") && v.MsgRct.ExitCode == 0 {
			messages = append(messages, v)
		}
	}
	if len(messages) == 0 {
		return
	}
	
	var runners []parallel.Runner[[]*Event]
	
	for _, v := range messages {
		trace := v
		runners = append(runners, func(c context.Context) ([]*Event, error) {
			r, e := p.prepareTraceEvents(c, trace)
			if e != nil {
				return nil, e
			}
			return r, nil
		})
	}
	
	results := parallel.Run[[]*Event](ctx, 50, runners)
	if err != nil {
		return
	}
	err = parallel.Wait[[]*Event](results, func(v []*Event) error {
		p.events = append(p.events, v...)
		return nil
	})
	if err != nil {
		return
	}
	
	events = p.events
	
	return
}

func (p *Events) prepareTraceEvents(ctx context.Context, trace *londobell.TraceMessage) (events []*Event, err error) {
	
	var cid string
	
	defer func() {
		if err != nil {
			err = fmt.Errorf("prepare cid: %s event error: %s", cid, err)
		}
	}()
	
	if trace.SignedCid != nil && *trace.SignedCid != "" {
		cid = *trace.SignedCid
	} else {
		cid = trace.Cid
	}
	
	tx, err := p.agg.GetTransactionByCid(ctx, cid)
	if err != nil {
		return
	}
	if tx == nil {
		err = fmt.Errorf("expect eth transation, got nil")
		return
	}
	
	receipt, err := p.agg.GetTransactionReceiptByCid(ctx, cid)
	if err != nil {
		return
	}
	if receipt == nil {
		err = fmt.Errorf("expect eth transation, got nil")
		return
	}
	
	for _, v := range receipt.Logs {
		events = append(events, &Event{
			EthReceiptLog: v,
			TX:            tx,
			Receipt:       receipt,
			XCid:          cid,
		})
	}
	
	return
}
