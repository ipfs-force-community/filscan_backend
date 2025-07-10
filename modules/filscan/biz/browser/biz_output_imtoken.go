package browser

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gozelle/async/parallel"
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
	transferList := transfers.TracesByAccountIDList

	cidMethod := make(map[string]string)
	uniq := make(map[string]struct{})
	var runners []parallel.Runner[filscan.MessageByCidResponse]
	for _, transfer := range transferList {
		cid := transfer.Cid
		if cid != "" {
			cidMethod[cid] = transfer.MethodName
			runners = append(runners, func(_ context.Context) (filscan.MessageByCidResponse, error) {
				return i.MessageByCid(ctx, filscan.MessageByCidRequest{Cid: cid})
			})
		}
	}
	ch := parallel.Run[filscan.MessageByCidResponse](ctx, 50, runners)
	err = parallel.Wait[filscan.MessageByCidResponse](ch, func(v filscan.MessageByCidResponse) error {
		if v.Message != nil {
			if _, ok := uniq[v.Message.Cid]; !ok {
				uniq[v.Message.Cid] = struct{}{}
				resp.MessageList = append(resp.MessageList, v.Message)
			}
		}
		return nil
	})
	sort.Slice(resp.MessageList, func(i, j int) bool {
		if resp.MessageList[i].Epoch != resp.MessageList[j].Epoch {
			return resp.MessageList[i].Epoch > resp.MessageList[j].Epoch
		} else {
			return resp.MessageList[i].Nonce > resp.MessageList[j].Nonce
		}
	})
	resp.MessageList = resp.MessageList[req.RowId:]
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
