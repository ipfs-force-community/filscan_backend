package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/gozelle/fastjson"
	"github.com/tidwall/gjson"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

var _ londobell.Adapter = LondobellAdapterImpl{}

func NewLondobellAdapterImpl(host string, client *resty.Client) *LondobellAdapterImpl {
	return &LondobellAdapterImpl{host: host, client: client}
}

type LondobellAdapterImpl struct {
	host   string
	client *resty.Client
}

func (l LondobellAdapterImpl) exec(ctx context.Context, path string, body map[string]interface{}) (*resty.Response, error) {
	r := l.client.R()
	r.SetContext(ctx)
	r.SetBody(body)

	var err error
	defer func() {
		if err != nil {
			log.Errorf("request: %s error: %s", path, err)
		}
	}()
	url := fmt.Sprintf("%s/%s", strings.TrimRight(l.host, "/"), strings.TrimLeft(path, "/"))
	reply, err := r.Post(url)
	if err != nil {
		fmt.Printf("Post: %s error: %s\n", url, err)
		return nil, err
	}

	j, err := fastjson.ParseBytes(reply.Body())
	if err != nil {
		return nil, err
	}

	code := j.GetInt64("code")
	if code != 0 {
		return nil, fmt.Errorf(string(j.GetStringBytes("msg")))
	}

	return reply, nil
}

func (l LondobellAdapterImpl) bindResult(resp *resty.Response, result interface{}) (err error) {
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
	if gjson.Get(resp.String(), "code").String() != "0" {
		fmt.Println(resp.String())
		fmt.Println(fmt.Printf("request [%s]%s error: %s", resp.Request.Method, resp.Request.URL, gjson.Get(resp.String(), "msg")))
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

func (l LondobellAdapterImpl) Epoch(ctx context.Context, epoch *chain.Epoch) (reply *londobell.EpochReply, err error) {
	resp, err := l.exec(ctx, "/adapter/epoch", map[string]interface{}{
		"epoch": epoch,
	})
	if err != nil {
		return
	}
	reply = new(londobell.EpochReply)
	err = l.bindResult(resp, reply)
	if err != nil {
		return
	}
	return
}

func (l LondobellAdapterImpl) Actor(ctx context.Context, actorId chain.SmartAddress, epoch *chain.Epoch) (result *londobell.ActorState, err error) {
	resp, err := l.exec(ctx, "/adapter/actor", map[string]interface{}{
		"actorId": actorId.Address(),
		"epoch":   epoch,
	})
	if err != nil {
		return
	}
	result = new(londobell.ActorState)
	err = l.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}

func (l LondobellAdapterImpl) Miner(ctx context.Context, miner chain.SmartAddress, epoch *chain.Epoch) (result *londobell.MinerDetail, err error) {
	resp, err := l.exec(ctx, "/adapter/miner", map[string]interface{}{
		"miner": miner.Address(),
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

func (l LondobellAdapterImpl) ActorIDs(ctx context.Context, epoch *chain.Epoch) (result *londobell.ActorIDs, err error) {
	resp, err := l.exec(ctx, "/adapter/actor_ids", map[string]interface{}{
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

//func (l LondobellAdapterImpl) MessagePool(ctx context.Context, cid string, filters *types.Filters) (result *londobell.MessagePool, err error) {
//	var resp *resty.Response
//	if cid == "" {
//		resp, err = l.exec(ctx, "/adapter/mpool", map[string]interface{}{
//			"index":       filters.Index,
//			"limit":       filters.Limit,
//			"method_name": filters.MethodName,
//		})
//		if err != nil {
//			return
//		}
//	} else {
//		if regexp.MustCompile("^0x[0-9a-fA-F]{64}$").MatchString(cid) {
//			resp, err = l.exec(ctx, "/adapter/mpool", map[string]interface{}{
//				"hash": cid,
//			})
//			if err != nil {
//				return
//			}
//		} else {
//			resp, err = l.exec(ctx, "/adapter/mpool", map[string]interface{}{
//				"cid": cid,
//			})
//			if err != nil {
//				return
//			}
//		}
//	}
//	err = l.bindResult(resp, &result)
//	if err != nil {
//		return
//	}
//	return
//}

func (l LondobellAdapterImpl) MinerList(ctx context.Context, epoch *chain.Epoch) (result *londobell.MinerList, err error) {
	resp, err := l.exec(ctx, "/adapter/list_miners", map[string]interface{}{
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

func (l LondobellAdapterImpl) CurrentSectorInitialPledge(ctx context.Context, epoch *chain.Epoch) (result *londobell.CurrentSectorInitialPledge, err error) {
	resp, err := l.exec(ctx, "/adapter/current_sector_initial_pledge", map[string]interface{}{
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

func (l LondobellAdapterImpl) ChangeActors(ctx context.Context, epoch chain.Epoch) (reply map[string]*londobell.ChangeActorsReply, err error) {
	resp, err := l.exec(ctx, "/adapter/changed_actors", map[string]interface{}{
		"epoch": epoch,
	})
	if err != nil {
		return
	}
	reply = map[string]*londobell.ChangeActorsReply{}
	err = l.bindResult(resp, &reply)
	if err != nil {
		return
	}
	return
}

func (l LondobellAdapterImpl) LastEpoch(ctx context.Context, epoch chain.Epoch) (reply *londobell.EpochReply, err error) {
	resp, err := l.exec(ctx, "/adapter/last_epoch", map[string]interface{}{
		"epoch": epoch,
	})
	if err != nil {
		return
	}
	reply = new(londobell.EpochReply)
	err = l.bindResult(resp, reply)
	if err != nil {
		return
	}
	return
}

func (l LondobellAdapterImpl) InitCodeForEVM(ctx context.Context, actorId chain.SmartAddress) (result *londobell.EVMByteCode, err error) {
	resp, err := l.exec(ctx, "/adapter/initcode_for_evm", map[string]interface{}{
		"actorId": actorId,
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

func (l LondobellAdapterImpl) BalanceAtMarket(ctx context.Context, miners []chain.SmartAddress, epoch chain.Epoch) (balances []*londobell.MarketBalance, err error) {
	var addrs []string
	for _, v := range miners {
		addrs = append(addrs, v.Address())
	}
	resp, err := l.exec(ctx, "/adapter/balance_at_market", map[string]interface{}{
		"addrs": addrs,
		"epoch": epoch.Int64(),
	})
	if err != nil {
		return
	}
	err = l.bindResult(resp, &balances)
	if err != nil {
		return
	}
	return
}

func (l LondobellAdapterImpl) ActiveSectors(ctx context.Context, miner chain.SmartAddress, epoch chain.Epoch) (r *londobell.ActiveSectorsReply, err error) {
	resp, err := l.exec(ctx, "/adapter/active_sectors", map[string]interface{}{
		"actorId": miner.Address(),
		"epoch":   epoch.Int64(),
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
