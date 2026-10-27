package browser

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gozelle/async/parallel"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

func NewIMTokenBiz(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB, conf *config.Config) *IMTokenBiz {
	return &IMTokenBiz{
		agg:               agg,
		adapter:           adapter,
		BlockChainAclImpl: acl.NewBlockChainAclImpl(agg, adapter, dal.NewNFTQueryer(db), conf),
		AccountAclImpl:    acl.NewAccountAclImpl(agg, adapter),
	}
}

var _ filscan.IMToken = (*IMTokenBiz)(nil)

type IMTokenBiz struct {
	agg     londobell.Agg
	adapter londobell.Adapter
	*acl.BlockChainAclImpl
	*acl.AccountAclImpl
}

func (i IMTokenBiz) MessageByEpoch(ctx context.Context, req filscan.MessageListRequest) (resp filscan.MessageListResponse, err error) {
	height := types.InputType("height")
	startEpoch := chain.Epoch(req.Epoch)
	endEpoch := chain.Epoch(req.Epoch) + 1
	finalHeight, err := i.agg.LatestTipset(ctx)
	if err != nil {
		return
	}
	if finalHeight == nil {
		err = fmt.Errorf("latest tipset is empty")
		return
	}
	if int64(req.Epoch) > finalHeight[0].ID {
		err = fmt.Errorf("epoch exceed current latest height")
		return
	}

	blocks, err := i.agg.BlockHeader(ctx, types.Filters{
		InputType: &height,
		Start:     &startEpoch,
		End:       &endEpoch,
	})
	if err != nil {
		return
	}
	var filterTypeMap = make(map[string]bool)
	if req.Type != "" {
		split := strings.Split(req.Type, ",")
		for _, s := range split {
			if s == "InvokeEVM" {
				s = ""
			}
			filterTypeMap[s] = true
		}
	} else {
		filterTypeMap["Send"] = true
		filterTypeMap["Send(placeholder)"] = true
	}
	//var blockResult []*filscan.BlockIMToken
	var messageResult []*filscan.MessageIMToken
	var baseFee decimal.Decimal
	if len(blocks) != 0 {
		tipset, err := i.agg.ParentTipset(ctx, chain.Epoch(blocks[0].Epoch+1))
		if err != nil {
			return resp, err
		}
		baseFee = tipset[0].BaseFee
	}
	var runners []parallel.Runner[[]*londobell.MessageTrace]
	duplicateMap := make(map[string]struct{})
	for _, block := range blocks {
		var blockMessages []*londobell.MessageTrace
		blockMessages, err = i.agg.MessagesForBlock(ctx, block.ID, types.Filters{})
		if err != nil {
			return
		}
		for _, message := range blockMessages {
			if filterTypeMap[message.Method] {
				if _, ok := duplicateMap[message.Cid]; !ok {
					duplicateMap[message.Cid] = struct{}{}
					cid := message.Cid
					runners = append(runners, func(_ context.Context) ([]*londobell.MessageTrace, error) {
						return i.agg.TraceForMessage(ctx, cid)
					})
				}
			}
		}
	}
	ch := parallel.Run[[]*londobell.MessageTrace](ctx, 10, runners)
	err = parallel.Wait[[]*londobell.MessageTrace](ch, func(v []*londobell.MessageTrace) error {
		var res []*filscan.MessageIMToken
		for _, trace := range v {
			tk := assembler.IMToken{}.MessageToIMToken(trace, baseFee)
			if tk.To[1] == '0' {
				actorState, err := i.adapter.Actor(ctx, chain.SmartAddress(tk.To), nil)
				if err != nil {
					return err
				}
				tk.To = actorState.ActorAddr
			} else if tk.From[1] == '0' {
				actorState, err := i.adapter.Actor(ctx, chain.SmartAddress(tk.From), nil)
				if err != nil {
					return err
				}
				tk.From = actorState.ActorAddr
			}
			res = append(res, tk)
		}
		messageResult = append(messageResult, res...)
		return nil
	})
	if err != nil {
		return
	}
	resp = filscan.MessageListResponse{
		MessageList: assembler.IMToken{}.MessageListToIMToken(messageResult),
	}
	sort.Slice(resp.MessageList, func(i, j int) bool {
		return resp.MessageList[i].Cid < resp.MessageList[j].Cid

	})
	return
}

func (i IMTokenBiz) MessageByCid(ctx context.Context, req filscan.MessageByCidRequest) (resp filscan.MessageByCidResponse, err error) {
	var messageResult *filscan.MessageIMToken
	var messageDetail *londobell.MessageTrace
	messageDetails, err := i.agg.TraceForMessage(ctx, req.Cid)
	if err != nil {
		return
	}
	var baseFee decimal.Decimal
	if messageDetails != nil {
		messageDetail = messageDetails[0]
		if messageDetail.To.CrudeAddress()[0] == '0' {
			actorState, err := i.adapter.Actor(ctx, messageDetail.To, nil)
			if err != nil {
				return resp, err
			}
			messageDetail.To = chain.SmartAddress(actorState.ActorAddr)
		} else if messageDetail.From.CrudeAddress()[0] == '0' {
			actorState, err := i.adapter.Actor(ctx, messageDetail.From, nil)
			if err != nil {
				return resp, err
			}
			messageDetail.From = chain.SmartAddress(actorState.ActorAddr)
		}
		tipset, err := i.agg.ParentTipset(ctx, messageDetail.Epoch)
		if err != nil {
			return resp, err
		}
		baseFee = tipset[0].BaseFee
		messageResult = assembler.IMToken{}.MessageToIMToken(messageDetail, baseFee)
	}
	resp = filscan.MessageByCidResponse{
		Message: messageResult,
	}
	return
}

func (i IMTokenBiz) ChainMessages(ctx context.Context, req filscan.ChainMessagesRequest) (resp filscan.ChainMessagesResponse, err error) {
	limit := int64(50)
	var epoch chain.Epoch
	if req.Epoch == 0 {
		finalHeight, err := i.agg.LatestTipset(ctx)
		if err != nil {
			return resp, err
		}
		if finalHeight == nil {
			err = fmt.Errorf("latest tipset is empty")
			return resp, err
		} else {
			epoch = chain.Epoch(finalHeight[0].ID)
		}
	} else {
		epoch = chain.Epoch(req.Epoch)
	}
	transfers, err := i.GetActorTransfersForIMToken(ctx, chain.SmartAddress(req.Address), types.Filters{Limit: limit + 20, End: &epoch})
	if err != nil || transfers == nil {
		return
	}
	// 旧实现：每条流水各发一次 MessageByCid（内部 trace_for_message + parent_tipset +
	// 可能的 actor_id 解析），50 条消息实测打出 72 次 trace_for_message + 56 次
	// parent_tipset + 1 次 transfer_messages_by_epoch ≈ 129 次聚合器调用（50 路并发）。
	// 聚合器所有分片查询共享一个全局闸门（生产 limit=64，取不到令牌等 5s 后显式报错），
	// 于是单个用户请求就能把闸门吃光 ⇒ 5s 排队 ⇒ 20~60s 尾延迟 ⇒ 撞 60s 超时。
	// 现改为：cid 去重 → 按 epoch 批量取 trace → baseFee/actor 地址按 epoch/地址去重。
	cidOrder, cidEpoch, cidMethod := imTokenCids(transfers.TracesByAccountIDList)
	var traces map[string]*londobell.MessageTrace
	traces, err = i.imTokenTraces(ctx, cidOrder, cidEpoch)
	if err != nil {
		return resp, err
	}
	var baseFees map[int64]decimal.Decimal
	baseFees, err = i.imTokenBaseFees(ctx, traces)
	if err != nil {
		return resp, err
	}
	var resolved map[string]string
	resolved, err = i.imTokenResolveActors(ctx, traces)
	if err != nil {
		return resp, err
	}

	uniq := make(map[string]struct{}, len(cidOrder))
	for _, cid := range cidOrder {
		cached, ok := traces[cid]
		if !ok || cached == nil {
			continue
		}
		// 复制一份再用：trace 可能来自缓存，下面要改写 From/To，不能污染缓存
		trace := *cached
		imTokenApplyActorAddress(&trace, resolved)
		if _, ok := uniq[trace.Cid]; ok {
			continue
		}
		uniq[trace.Cid] = struct{}{}
		message := assembler.IMToken{}.MessageToIMToken(&trace, baseFees[trace.Epoch.Int64()])
		if message == nil {
			continue
		}
		resp.MessageList = append(resp.MessageList, message)
	}
	sort.Slice(resp.MessageList, func(i, j int) bool {
		if resp.MessageList[i].Epoch != resp.MessageList[j].Epoch {
			return resp.MessageList[i].Epoch > resp.MessageList[j].Epoch
		}
		if resp.MessageList[i].Nonce != resp.MessageList[j].Nonce {
			return resp.MessageList[i].Nonce > resp.MessageList[j].Nonce
		}
		// 同 epoch 同 nonce（不同发送方）时给出稳定顺序；旧实现是并发完成序，本身不确定
		return resp.MessageList[i].Cid < resp.MessageList[j].Cid
	})
	if req.RowId > 0 {
		if req.RowId >= int64(len(resp.MessageList)) {
			// 旧实现这里会 slice 越界 panic；游标越界按空页返回
			resp.MessageList = nil
			return resp, nil
		}
		resp.MessageList = resp.MessageList[req.RowId:]
	}
	if len(resp.MessageList) != 0 {
		if resp.MessageList[0].Cid != "" {
			resp.MessageList[0].Method = cidMethod[resp.MessageList[0].Cid]
		}
		resp.MessageList[0].RowId = 1
		//当请求req不为0，且rowid不为0时候
		if req.Epoch != 0 && req.RowId != 0 && resp.MessageList[0].Epoch == req.Epoch {
			resp.MessageList[0].RowId = req.RowId + 1
		}
	}
	for index := 1; index < len(resp.MessageList); index++ {
		//给每个消息打标签，避免通过cid找消息将receive这种变成了send
		messageIMToken := resp.MessageList[index]
		if messageIMToken.Cid != "" {
			messageIMToken.Method = cidMethod[messageIMToken.Cid]
		}
		if resp.MessageList[index].Epoch == resp.MessageList[index-1].Epoch {
			resp.MessageList[index].RowId = resp.MessageList[index-1].RowId + 1
		} else {
			resp.MessageList[index].RowId = 1
		}
	}
	if int(limit) < len(resp.MessageList) {
		resp.MessageList = resp.MessageList[:limit]
	}
	return
}

// imTokenTraceConcurrency 是 /api/v1/ChainMessages 取数（批量 trace / parent_tipset /
// actor 地址解析）的并发上限。
//
// 依据：这个接口的下游调用量必须远小于聚合器的全局分片查询闸门（生产 limit=64，
// 每条分片查询都要拿令牌，拿不到等 5s 后显式报错），否则单个用户请求就能把闸门占满，
// 让所有接口一起排队、一起失败。旧实现是 50 路并发、每路 3 次调用，实测一次请求就打出
// 129 次下游调用；8 路把单次请求压到几十毫秒，同时只占闸门的 1/8。
const imTokenTraceConcurrency = 8

// imTokenFallbackTraceConcurrency 是批量取 trace 没覆盖到时逐条回退的并发上限（更保守）。
const imTokenFallbackTraceConcurrency = 4

// imTokenCids 把流水列表压成「去重后的 cid 序列 + 每个 cid 的 epoch + 展示用方法名」。
//
// 方法名沿用旧实现「后写覆盖」的语义；cid 顺序按首次出现，保证下游回退时的顺序稳定。
func imTokenCids(transferList []*filscan.MessageBasic) (cidOrder []string, cidEpoch map[string]int64, cidMethod map[string]string) {
	cidEpoch = make(map[string]int64, len(transferList))
	cidMethod = make(map[string]string, len(transferList))
	cidOrder = make([]string, 0, len(transferList))
	for _, transfer := range transferList {
		if transfer == nil || transfer.Cid == "" {
			continue
		}
		cidMethod[transfer.Cid] = transfer.MethodName
		if _, ok := cidEpoch[transfer.Cid]; ok {
			continue
		}
		if transfer.Height != nil {
			cidEpoch[transfer.Cid] = transfer.Height.Int64()
		} else {
			cidEpoch[transfer.Cid] = 0 // 没有高度就只能靠逐条回退取
		}
		cidOrder = append(cidOrder, transfer.Cid)
	}
	return
}

// 链上数据里有两类东西一旦落链就不再变化：一条消息的 trace（按 cid 唯一）、
// 一个 epoch 的父 tipset baseFee。ChainMessages 被客户端按地址高频轮询（同一个地址
// 每 1~2 秒一次），每次都把这几十条消息重新查一遍完全是浪费——而每次查询都要占用
// 聚合器的全局分片查询闸门。这里给「不可变数据」加一层有界 TTL 缓存：
// 地址流水列表（会变）不缓存，新消息仍要实时查，因此不改变可见性语义。
const (
	imTokenTraceCacheSize = 200_000
	imTokenTraceCacheTTL  = 10 * time.Minute
)

var (
	imTokenTraceCache   = expirable.NewLRU[string, *londobell.MessageTrace](imTokenTraceCacheSize, nil, imTokenTraceCacheTTL)
	imTokenBaseFeeCache = expirable.NewLRU[int64, decimal.Decimal](imTokenTraceCacheSize, nil, imTokenTraceCacheTTL)
	// actor id → 地址 的映射同样是不可变的（一个 ID 永远对应同一个地址）
	imTokenActorCache = expirable.NewLRU[string, string](imTokenTraceCacheSize, nil, 30*time.Minute)
)

func imTokenResetCaches() {
	imTokenTraceCache.Purge()
	imTokenBaseFeeCache.Purge()
	imTokenActorCache.Purge()
}

// imTokenTraces 取每个 cid 的 trace：先查不可变数据缓存，未命中的按 epoch 分组合成
// /aggregators/batch_trace_for_message；批量没覆盖到（或批量端点不可用）的 cid 再逐条回退，
// 保证与旧实现「每条 cid 各取一次 trace_for_message」的结果一致。
func (i IMTokenBiz) imTokenTraces(ctx context.Context, cidOrder []string, cidEpoch map[string]int64) (map[string]*londobell.MessageTrace, error) {
	traces := make(map[string]*londobell.MessageTrace, len(cidOrder))
	if len(cidOrder) == 0 {
		return traces, nil
	}

	misses := make([]string, 0, len(cidOrder))
	for _, cid := range cidOrder {
		if trace, ok := imTokenTraceCache.Get(cid); ok && trace != nil {
			traces[cid] = trace
			continue
		}
		misses = append(misses, cid)
	}
	if len(misses) == 0 {
		return traces, nil
	}

	fetched, err := i.imTokenFetchTraces(ctx, misses, cidEpoch)
	if err != nil {
		return nil, err
	}
	for cid, trace := range fetched {
		traces[cid] = trace
		if trace != nil {
			imTokenTraceCache.Add(cid, trace)
		}
	}
	return traces, nil
}

func (i IMTokenBiz) imTokenFetchTraces(ctx context.Context, cidOrder []string, cidEpoch map[string]int64) (map[string]*londobell.MessageTrace, error) {
	byEpoch := make(map[int64][]string, len(cidOrder))
	for _, cid := range cidOrder {
		epoch := cidEpoch[cid]
		byEpoch[epoch] = append(byEpoch[epoch], cid)
	}

	type groupResult struct {
		epoch  int64
		cids   []string
		traces []*londobell.MessageTrace
		err    error
	}

	var runners []parallel.Runner[groupResult]
	for epoch, cids := range byEpoch {
		epoch, cids := epoch, cids
		runners = append(runners, func(_ context.Context) (groupResult, error) {
			traces, err := i.agg.BatchTraceForMessage(ctx, chain.Epoch(epoch), cids)
			return groupResult{epoch: epoch, cids: cids, traces: traces, err: err}, nil
		})
	}

	traces := make(map[string]*londobell.MessageTrace, len(cidOrder))
	var mu sync.Mutex
	ch := parallel.Run[groupResult](ctx, imTokenTraceConcurrency, runners)
	err := parallel.Wait[groupResult](ch, func(v groupResult) error {
		if v.err != nil {
			log.Warnf("imtoken ChainMessages: batch trace failed (epoch=%d cids=%d): %s; fallback to per-cid trace", v.epoch, len(v.cids), v.err)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, trace := range v.traces {
			if trace == nil || trace.Cid == "" {
				continue
			}
			if _, ok := traces[trace.Cid]; !ok {
				traces[trace.Cid] = trace
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var missing []string
	for _, cid := range cidOrder {
		if _, ok := traces[cid]; !ok {
			missing = append(missing, cid)
		}
	}
	if len(missing) == 0 {
		return traces, nil
	}

	log.Warnf("imtoken ChainMessages: %d/%d cids missing from batch trace, fallback to per-cid", len(missing), len(cidOrder))
	var fallback []parallel.Runner[*londobell.MessageTrace]
	for _, cid := range missing {
		cid := cid
		fallback = append(fallback, func(_ context.Context) (*londobell.MessageTrace, error) {
			list, err := i.agg.TraceForMessage(ctx, cid)
			if err != nil {
				return nil, err
			}
			if len(list) == 0 {
				return nil, nil
			}
			return list[0], nil // 与 MessageByCid 一致：只取第一条
		})
	}
	chFallback := parallel.Run[*londobell.MessageTrace](ctx, imTokenFallbackTraceConcurrency, fallback)
	err = parallel.Wait[*londobell.MessageTrace](chFallback, func(trace *londobell.MessageTrace) error {
		if trace == nil || trace.Cid == "" {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		traces[trace.Cid] = trace
		return nil
	})
	if err != nil {
		return nil, err
	}
	return traces, nil
}

// imTokenBaseFees 取每个 epoch 的父 tipset baseFee：同一 epoch 只查一次，且落链后不再变化
// （旧实现每条消息各查一次 parent_tipset，50 条消息实测 56 次）。
func (i IMTokenBiz) imTokenBaseFees(ctx context.Context, traces map[string]*londobell.MessageTrace) (map[int64]decimal.Decimal, error) {
	epochs := make(map[int64]struct{}, len(traces))
	var epochList []int64
	for _, trace := range traces {
		if trace == nil {
			continue
		}
		epoch := trace.Epoch.Int64()
		if _, ok := epochs[epoch]; ok {
			continue
		}
		epochs[epoch] = struct{}{}
		epochList = append(epochList, epoch)
	}

	fees := make(map[int64]decimal.Decimal, len(epochList))
	var misses []int64
	for _, epoch := range epochList {
		if fee, ok := imTokenBaseFeeCache.Get(epoch); ok {
			fees[epoch] = fee
			continue
		}
		misses = append(misses, epoch)
	}
	if len(misses) == 0 {
		return fees, nil
	}
	sort.Slice(misses, func(a, b int) bool { return misses[a] < misses[b] })

	type feeResult struct {
		epoch int64
		fee   decimal.Decimal
	}
	var runners []parallel.Runner[feeResult]
	for _, epoch := range misses {
		epoch := epoch
		runners = append(runners, func(_ context.Context) (feeResult, error) {
			tipset, err := i.agg.ParentTipset(ctx, chain.Epoch(epoch))
			if err != nil {
				return feeResult{}, err
			}
			if len(tipset) == 0 {
				return feeResult{}, fmt.Errorf("parent tipset of epoch %d is empty", epoch)
			}
			return feeResult{epoch: epoch, fee: tipset[0].BaseFee}, nil
		})
	}

	var mu sync.Mutex
	ch := parallel.Run[feeResult](ctx, imTokenTraceConcurrency, runners)
	err := parallel.Wait[feeResult](ch, func(v feeResult) error {
		mu.Lock()
		defer mu.Unlock()
		fees[v.epoch] = v.fee
		imTokenBaseFeeCache.Add(v.epoch, v.fee)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fees, nil
}

// imTokenResolveActors 解析需要换成 0x/f 地址的 actor id：同一地址只解析一次
// （旧实现每条消息各解析一次）。
func (i IMTokenBiz) imTokenResolveActors(ctx context.Context, traces map[string]*londobell.MessageTrace) (map[string]string, error) {
	targets := make(map[string]chain.SmartAddress)
	var targetList []string
	for _, trace := range traces {
		if trace == nil {
			continue
		}
		// 与 MessageByCid 一致：To 需要解析时不再看 From
		addr := trace.To
		if !imTokenIsActorID(addr) {
			addr = trace.From
			if !imTokenIsActorID(addr) {
				continue
			}
		}
		key := string(addr)
		if _, ok := targets[key]; ok {
			continue
		}
		targets[key] = addr
		targetList = append(targetList, key)
	}
	sort.Strings(targetList)

	type actorResult struct {
		key  string
		addr string
	}

	resolved := make(map[string]string, len(targetList))
	var misses []string
	for _, key := range targetList {
		if addr, ok := imTokenActorCache.Get(key); ok {
			resolved[key] = addr
			continue
		}
		misses = append(misses, key)
	}
	if len(misses) == 0 {
		return resolved, nil
	}

	var runners []parallel.Runner[actorResult]
	for _, key := range misses {
		key := key
		addr := targets[key]
		runners = append(runners, func(_ context.Context) (actorResult, error) {
			state, err := i.adapter.Actor(ctx, addr, nil)
			if err != nil {
				return actorResult{}, err
			}
			return actorResult{key: key, addr: state.ActorAddr}, nil
		})
	}

	var mu sync.Mutex
	ch := parallel.Run[actorResult](ctx, imTokenTraceConcurrency, runners)
	err := parallel.Wait[actorResult](ch, func(v actorResult) error {
		mu.Lock()
		defer mu.Unlock()
		resolved[v.key] = v.addr
		imTokenActorCache.Add(v.key, v.addr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// imTokenApplyActorAddress 把 trace 里的 actor id 换成解析后的地址。规则与 MessageByCid
// 逐字一致（To 优先，且 To 需要解析时不看 From）。
func imTokenApplyActorAddress(trace *londobell.MessageTrace, resolved map[string]string) {
	if imTokenIsActorID(trace.To) {
		if addr, ok := resolved[string(trace.To)]; ok {
			trace.To = chain.SmartAddress(addr)
		}
		return
	}
	if imTokenIsActorID(trace.From) {
		if addr, ok := resolved[string(trace.From)]; ok {
			trace.From = chain.SmartAddress(addr)
		}
	}
}

// imTokenIsActorID 判断地址是否是 actor id（crude 首字符 '0'，即 "01234" 这类）。
// 旧实现直接 CrudeAddress()[0]，空地址会 panic；这里先判长度。
func imTokenIsActorID(addr chain.SmartAddress) bool {
	crude := addr.CrudeAddress()
	return len(crude) > 0 && crude[0] == '0'
}
