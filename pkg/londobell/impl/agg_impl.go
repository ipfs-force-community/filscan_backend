package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	logging "github.com/gozelle/logger"
	"github.com/tidwall/gjson"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

var _ londobell.Agg = LondobellAggImpl{}

var log = logging.NewLogger("agg")

var ErrNotFound = fmt.Errorf("error not found")

func NewLondobellAggImpl(host string, client *resty.Client) *LondobellAggImpl {
	return &LondobellAggImpl{host: host, client: client}
}

type LondobellAggImpl struct {
	host   string
	client *resty.Client
}

func (l LondobellAggImpl) exec(ctx context.Context, path string, body map[string]interface{}) (*resty.Response, error) {
	r := l.client.R()
	r.SetContext(ctx)
	r.SetBody(body)

	return r.Post(fmt.Sprintf("%s/%s", strings.TrimRight(l.host, "/"), strings.TrimLeft(path, "/")))
}

func (l LondobellAggImpl) bindResult(resp *resty.Response, result interface{}) (err error) {
	defer func() {
		if err != nil {
			result = nil
		}
	}()
	if reflect.TypeOf(result).Kind() != reflect.Ptr {
		err = fmt.Errorf("only accept pointer")
		return
	}
	if resp.Request == nil {
		resp.Request = &resty.Request{Method: "unknown", URL: "unknown"}
	}
	code := gjson.Get(resp.String(), "code").String()

	if code == "2" {
		err = ErrNotFound
		return
	}

	if code != "0" {
		log.Errorf("response: %s", resp.String())
		log.Errorf("request [%s]%s code: %s error: %s", resp.Request.Method, resp.Request.URL, code, gjson.Get(resp.String(), "msg"))
		err = fmt.Errorf(gjson.Get(resp.String(), "msg").String())
		return
	}
	data := gjson.Get(resp.String(), "data").Raw
	if len(data) == 0 {
		err = fmt.Errorf("empty data")
		return
	}
	err = json.Unmarshal([]byte(data), result)
	if err != nil {
		err = fmt.Errorf("unmarshal error:%s", err)
		return
	}
	return
}

func (l LondobellAggImpl) ChildCallsForMessage(ctx context.Context, cid string) (result []*londobell.InternalTransfer, err error) {
	resp, err := l.exec(ctx, "/aggregators/child_calls_for_message", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) Address(ctx context.Context, addr chain.SmartAddress) (result *londobell.Address, err error) {
	resp, err := l.exec(ctx, "/aggregators/address", map[string]interface{}{
		"addr": addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) AggPreNetFee(ctx context.Context, start chain.Epoch, end chain.Epoch) (result []*londobell.AggPreNetFee, err error) {
	resp, err := l.exec(ctx, "/aggregators/agg_pre_netfee", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) AggProNetFee(ctx context.Context, start chain.Epoch, end chain.Epoch) (result []*londobell.AggProNetFee, err error) {
	resp, err := l.exec(ctx, "/aggregators/agg_pro_netfee", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlockMessages(ctx context.Context, filters types.Filters) (result *londobell.BlockMessagesList, err error) {
	resp, err := l.exec(ctx, "/aggregators/block", map[string]interface{}{
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MinerBlockReward(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result []*londobell.MinerBlockReward, err error) {
	resp, err := l.exec(ctx, "/aggregators/miner_blockreward", map[string]interface{}{
		"addr":  addr.CrudeAddress(),
		"start": filters.Start,
		"end":   filters.End,
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) FinalHeight(ctx context.Context) (epoch *chain.Epoch, err error) {
	resp, err := l.exec(ctx, "/aggregators/final_height", map[string]interface{}{})
	if err != nil {
		return
	}

	type R struct {
		Epoch chain.Epoch
	}
	var result []R
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}

	if len(result) != 1 {
		err = fmt.Errorf("expect result length: 1, got %d", len(result))
		return
	}

	epoch = &result[0].Epoch
	return
}

func (l LondobellAggImpl) StateFinalHeight(ctx context.Context) (epoch *chain.Epoch, err error) {
	resp, err := l.exec(ctx, "/aggregators/state_final_height", map[string]interface{}{})
	if err != nil {
		return
	}

	type R struct {
		Epoch chain.Epoch
	}
	var result []R
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}

	if len(result) != 1 {
		err = fmt.Errorf("expect result length: 1, got %d", len(result))
		return
	}

	epoch = &result[0].Epoch
	return
}

func (l LondobellAggImpl) MinersInfo(ctx context.Context, start, end chain.Epoch) (r []*londobell.MinerInfo, err error) {
	resp, err := l.exec(ctx, "/aggregators/miners_info", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &r)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) WinCount(ctx context.Context, start, end chain.Epoch) (result []*londobell.MinerWinCount, err error) {
	resp, err := l.exec(ctx, "/aggregators/wincount", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) Traces(ctx context.Context, start, end chain.Epoch) (r []*londobell.TraceMessage, err error) {
	resp, err := l.exec(ctx, "/aggregators/traces", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &r)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MinersBlockReward(ctx context.Context, start chain.Epoch, end chain.Epoch) (result []*londobell.MinersBlockReward, err error) {
	resp, err := l.exec(ctx, "/aggregators/miners_blockreward", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) LatestTipset(ctx context.Context) (result []*londobell.Tipset, err error) {
	resp, err := l.exec(ctx, "/aggregators/latest_tipset", map[string]interface{}{})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ActorStateEpoch(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) (result []*londobell.ActorStateEpoch, err error) {
	resp, err := l.exec(ctx, "/aggregators/actor_state_epoch", map[string]interface{}{
		"start": epoch,
		"addr":  addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) Tipset(ctx context.Context, epoch chain.Epoch) (result []*londobell.Tipset, err error) {
	resp, err := l.exec(ctx, "/aggregators/tipset", map[string]interface{}{
		"start": epoch,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TipsetsList(ctx context.Context, filters types.Filters) (result *londobell.TipsetsList, err error) {
	resp, err := l.exec(ctx, "/aggregators/tipsets_list", map[string]interface{}{
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) GetFilsupply(ctx context.Context, epochs []chain.Epoch) (result []*londobell.CirculatingSupply, err error) {
	resp, err := l.exec(ctx, "/aggregators/get_filsupply", map[string]interface{}{
		"epochs": epochs,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ChangedActors(ctx context.Context, epoch chain.Epoch) (result []*londobell.ChangedActorRes, err error) {
	resp, err := l.exec(ctx, "/aggregators/changed_actors", map[string]interface{}{
		"epoch": epoch,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MinerInfo(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) (result []*londobell.MinerInfo, err error) {
	resp, err := l.exec(ctx, "/aggregators/miner_info", map[string]interface{}{
		"start": epoch,
		"addr":  addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ActorBalance(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) (result []*londobell.ActorBalance, err error) {
	resp, err := l.exec(ctx, "/aggregators/balance", map[string]interface{}{
		"start": epoch,
		"addr":  addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MinersForOwner(ctx context.Context, addr chain.SmartAddress) (result []*londobell.MinersOfOwner, err error) {
	resp, err := l.exec(ctx, "/aggregators/miners_for_owner", map[string]interface{}{
		"addr": addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ActorMessages(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result *londobell.ActorMessagesList, err error) {
	resp, err := l.exec(ctx, "/aggregators/messages_for_actor", map[string]interface{}{
		"addr":  addr.CrudeAddress(),
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TransferMessages(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result *londobell.TransferMessagesList, err error) {
	resp, err := l.exec(ctx, "/aggregators/transfer_messages", map[string]interface{}{
		"addr":          addr.CrudeAddress(),
		"index":         &filters.Index,
		"limit":         &filters.Limit,
		"transfer-type": filters.MethodName,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TransferMessagesByEpoch(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result *londobell.TransferMessagesList, err error) {
	resp, err := l.exec(ctx, "/aggregators/transfer_messages_by_epoch", map[string]interface{}{
		"addr":  addr.CrudeAddress(),
		"index": &filters.Index,
		"limit": &filters.Limit,
		"end":   filters.End.Int64(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MessagesForFund(ctx context.Context, start, end chain.Epoch) (result *londobell.TransferMessagesList, err error) {
	resp, err := l.exec(ctx, "/aggregators/messages_for_fund", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TimeOfTrace(ctx context.Context, addr chain.SmartAddress, sort int) (result []*londobell.TimeOfTrace, err error) {
	resp, err := l.exec(ctx, "/aggregators/time_of_trace", map[string]interface{}{
		"addr": addr.CrudeAddress(),
		"sort": sort,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MinerCreateTime(ctx context.Context, addr chain.SmartAddress, to string, method int) (result *londobell.TimeOfTrace, err error) {
	resp, err := l.exec(ctx, "/aggregators/create_time", map[string]interface{}{
		"addr":   addr.CrudeAddress(),
		"to":     to,
		"method": method,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MinerGasCost(ctx context.Context, start chain.Epoch, end chain.Epoch) (result []*londobell.MinerGasCost, err error) {
	resp, err := l.exec(ctx, "/aggregators/gascost_for_sector", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TransferLargeAmount(ctx context.Context, filters types.Filters) (result *londobell.TransferLargeAmountList, err error) {
	resp, err := l.exec(ctx, "/aggregators/transfer_message_for_largeAmount", map[string]interface{}{
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) DealsList(ctx context.Context, filters types.Filters) (result *londobell.DealsList, err error) {
	resp, err := l.exec(ctx, "/aggregators/deals", map[string]interface{}{
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) DealDetail(ctx context.Context, dealId int64) (result *londobell.DealDetail, err error) {
	resp, err := l.exec(ctx, "/aggregators/detail_for_deal", map[string]interface{}{
		"dealId": dealId,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlockHeader(ctx context.Context, filters types.Filters) (result []*londobell.BlockHeader, err error) {
	resp, err := l.exec(ctx, "/aggregators/blockheader", map[string]interface{}{
		"start": filters.Start,
		"end":   filters.End,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) IncomingBlockHeader(ctx context.Context, filters types.Filters) (result []*londobell.BlockHeader, err error) {
	resp, err := l.exec(ctx, "/aggregators/incoming_blockheader", map[string]interface{}{
		"start": filters.Start,
		"end":   filters.End,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) IncomingBlockHeaderByCid(ctx context.Context, cid string) (result []*londobell.BlockHeader, err error) {
	resp, err := l.exec(ctx, "/aggregators/incoming_blockheader_by_cid", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) Tipsets(ctx context.Context, filters types.Filters) (result []*londobell.Tipset, err error) {
	resp, err := l.exec(ctx, "/aggregators/tipsets", map[string]interface{}{
		"start": filters.Start,
		"end":   filters.End,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TraceForMessage(ctx context.Context, cid string) (result []*londobell.MessageTrace, err error) {
	resp, err := l.exec(ctx, "/aggregators/trace_for_message", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ChildTransfersForMessage(ctx context.Context, cid string) (result []*londobell.MessageTrace, err error) {
	resp, err := l.exec(ctx, "/aggregators/child_transfers_for_message", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) DealDetails(ctx context.Context, dealID int64) (result []*londobell.DealDetail, err error) {
	resp, err := l.exec(ctx, "/aggregators/detail_for_deal", map[string]interface{}{
		"id": dealID,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ParentTipset(ctx context.Context, start chain.Epoch) (result []*londobell.ParentTipset, err error) {
	resp, err := l.exec(ctx, "/aggregators/parent_tipset", map[string]interface{}{
		"start": start,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlockHeaderByCid(ctx context.Context, cid string) (result []*londobell.BlockHeader, err error) {
	resp, err := l.exec(ctx, "/aggregators/blockheader_by_cid", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlockMessagesByMethodName(ctx context.Context, filters types.Filters) (result *londobell.MessagesByMethodNameList, err error) {
	resp, err := l.exec(ctx, "/aggregators/blockmessages_by_methodname", map[string]interface{}{
		"index":       filters.Index,
		"limit":       filters.Limit,
		"method_name": filters.MethodName,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) ActorMessagesByMethodName(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result *londobell.MessagesByMethodNameList, err error) {
	resp, err := l.exec(ctx, "/aggregators/actormessages_by_methodname", map[string]interface{}{
		"addr":        addr.CrudeAddress(),
		"index":       filters.Index,
		"limit":       filters.Limit,
		"method_name": filters.MethodName,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlockHeadersByMiner(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result *londobell.BlockHeadersByMiner, err error) {
	resp, err := l.exec(ctx, "/aggregators/blockheaders_by_miner", map[string]interface{}{
		"addr":  addr.CrudeAddress(),
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) DealsByAddr(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (result *londobell.DealsByAddr, err error) {
	resp, err := l.exec(ctx, "/aggregators/deals_by_addr", map[string]interface{}{
		"addr":  addr.CrudeAddress(),
		"start": filters.Start,
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) AllMethods(ctx context.Context) (result []*londobell.MethodName, err error) {
	resp, err := l.exec(ctx, "/aggregators/all_methods", map[string]interface{}{})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) AllMethodsForActor(ctx context.Context, addr chain.SmartAddress) (result []*londobell.MethodName, err error) {
	resp, err := l.exec(ctx, "/aggregators/all_methods_for_actor", map[string]interface{}{
		"addr": addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlocksForMessage(ctx context.Context, cid string) (result []*londobell.BlockHeader, err error) {
	resp, err := l.exec(ctx, "/aggregators/blocks_for_message", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MessagesForBlock(ctx context.Context, cid string, filters types.Filters) (result []*londobell.MessageTrace, err error) {
	resp, err := l.exec(ctx, "/aggregators/messages_for_block", map[string]interface{}{
		"cid":   cid,
		"index": &filters.Index,
		"limit": &filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BatchTraceForMessage(ctx context.Context, start chain.Epoch, cids []string) (result []*londobell.MessageTrace, err error) {
	resp, err := l.exec(ctx, "/aggregators/batch_trace_for_message", map[string]interface{}{
		"start": start,
		"cids":  cids,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) RichList(ctx context.Context, filters types.Filters) (result *londobell.RichList, err error) {
	resp, err := l.exec(ctx, "/aggregators/richlist", map[string]interface{}{
		"start": filters.Start,
		"index": filters.Index,
		"limit": filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) AllMethodsForBlockMessage(ctx context.Context, cid string) (result []*londobell.MethodName, err error) {
	resp, err := l.exec(ctx, "/aggregators/count_and_methods_of_messages_for_blockheader", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MessagesForBlockByMethodName(ctx context.Context, cid string, filters types.Filters) (result *londobell.MessagesOfBlock, err error) {
	resp, err := l.exec(ctx, "/aggregators/blockheader_messages_by_methodname", map[string]interface{}{
		"cid":         cid,
		"start":       filters.Start,
		"method_name": filters.MethodName,
		"index":       filters.Index,
		"limit":       filters.Limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) DealByID(ctx context.Context, dealID int64) (result []*londobell.Deals, err error) {
	resp, err := l.exec(ctx, "aggregators/deal_by_id", map[string]interface{}{
		"id": dealID,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) CreateTime(ctx context.Context, addr chain.SmartAddress) (epoch chain.Epoch, err error) {
	resp, err := l.exec(ctx, "aggregators/create_time", map[string]interface{}{
		"addr": addr.CrudeAddress(),
	})
	if err != nil {
		return
	}

	type result struct {
		Epoch int64
	}
	r := &result{}
	err = l.bindResult(resp, r)
	if err != nil {
		return
	}

	epoch = chain.Epoch(r.Epoch)

	return
}

func (l LondobellAggImpl) CountOfBlockMessages(ctx context.Context, start, end chain.Epoch) (count int64, err error) {
	resp, err := l.exec(ctx, "aggregators/count_of_blockmessages", map[string]interface{}{
		"start": start.Int64(),
		"end":   end.Int64(),
	})
	if err != nil {
		return
	}

	type result struct {
		Count int64
	}
	r := &result{}
	err = l.bindResult(resp, r)
	if err != nil {
		return
	}

	count = r.Count

	return
}

func (l LondobellAggImpl) GetTransactionByCid(ctx context.Context, cid string) (tx *londobell.EthTransaction, err error) {
	resp, err := l.exec(ctx, "aggregators/get_transaction_by_cid", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}

	type result struct {
		EthTransaction *londobell.EthTransaction `json:"ethTransaction"`
	}
	r := &result{}
	err = l.bindResult(resp, r)
	if err != nil {
		return
	}

	tx = r.EthTransaction

	return
}

func (l LondobellAggImpl) GetTransactionReceiptByCid(ctx context.Context, cid string) (receipt *londobell.EthReceipt, err error) {
	resp, err := l.exec(ctx, "aggregators/get_transaction_receipt_by_cid", map[string]interface{}{
		"cid": cid,
	})
	if err != nil {
		return
	}

	type result struct {
		EthReceipt *londobell.EthReceipt `json:"ethReceipt"`
	}
	r := &result{}
	err = l.bindResult(resp, r)
	if err != nil {
		return
	}

	receipt = r.EthReceipt

	return
}

func (l LondobellAggImpl) GetEvmInitCodeByActorID(ctx context.Context, actorId chain.SmartAddress) (res *londobell.ActorInitCode, err error) {
	resp, err := l.exec(ctx, "aggregators/get_evm_initcode_by_actorID", map[string]interface{}{
		"addr": actorId.CrudeAddress(),
	})
	if err != nil {
		return
	}

	res = new(londobell.ActorInitCode)
	err = l.bindResult(resp, res)
	if err != nil {
		return
	}

	return
}

func (l LondobellAggImpl) MessageCidByHash(ctx context.Context, hash string) (result *londobell.CidOrHash, err error) {
	resp, err := l.exec(ctx, "aggregators/messagecid_by_hash", map[string]interface{}{
		"cid": hash,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) HashByMessageCid(ctx context.Context, messageCid string) (result *londobell.CidOrHash, err error) {
	resp, err := l.exec(ctx, "aggregators/hash_by_messagecid", map[string]interface{}{
		"cid": messageCid,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) BlockMessageForEpochRange(ctx context.Context, start, end chain.Epoch) ([]*londobell.BlockMessageCids, error) {
	resp, err := l.exec(ctx, "aggregators/blockmessages_for_epochrange", map[string]interface{}{
		"start": start,
		"end":   end,
	})
	if err != nil {
		return nil, err
	}
	result := []*londobell.BlockMessageCids{}
	err = l.bindResult(resp, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (l LondobellAggImpl) InitCodeForEvm(ctx context.Context, addr chain.SmartAddress) (result *londobell.InitCode, err error) {
	resp, err := l.exec(ctx, "aggregators/initcode_for_evm", map[string]interface{}{
		"addr": addr.CrudeAddress(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) EventsForActor(ctx context.Context, addr chain.SmartAddress, index int64, limit int64) (result *londobell.EventList, err error) {
	resp, err := l.exec(ctx, "aggregators/events_for_actor", map[string]interface{}{
		"addr":  addr.CrudeAddress(),
		"index": index,
		"limit": limit,
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) TestContractTransfer(ctx context.Context, start chain.Epoch, end chain.Epoch) (result []*londobell.TraceMessage, err error) {
	resp, err := l.exec(ctx, "aggregators/test_contract_transfer", map[string]interface{}{
		"start": start.Int64(),
		"end":   end.Int64(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) MessagePool(ctx context.Context, cid string, filters *types.Filters) (result *londobell.MessagePool, err error) {
	var resp *resty.Response
	if cid == "" {
		resp, err = l.exec(ctx, "/aggregators/mpool", map[string]interface{}{
			"index":       filters.Index,
			"limit":       filters.Limit,
			"method_name": filters.MethodName,
		})
		if err != nil {
			return
		}
	} else {
		if regexp.MustCompile("^0x[0-9a-fA-F]{64}$").MatchString(cid) {
			resp, err = l.exec(ctx, "/aggregators/mpool", map[string]interface{}{
				"hash": cid,
			})
			if err != nil {
				return
			}
		} else {
			resp, err = l.exec(ctx, "/aggregators/mpool", map[string]interface{}{
				"cid": cid,
			})
			if err != nil {
				return
			}
		}
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAggImpl) AllMethodsForMessagePool(ctx context.Context) (result []*londobell.MethodName, err error) {
	resp, err := l.exec(ctx, "/aggregators/allmethods_for_mpool", map[string]interface{}{})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}
