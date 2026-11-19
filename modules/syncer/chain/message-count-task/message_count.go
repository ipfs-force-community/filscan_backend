package message_count_task

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

func NewMessageCountTask(repo repository.MessageCountTaskRepo) *MessageCountTask {
	return &MessageCountTask{repo: repo}
}

var _ syncer.Task = (*MessageCountTask)(nil)

type MessageCountTask struct {
	repo repository.MessageCountTaskRepo
}

func (b MessageCountTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (b MessageCountTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = b.repo.DeleteMessageCounts(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (b MessageCountTask) Name() string {
	return "message-count-task"
}

func (b MessageCountTask) Exec(ctx *syncer.Context) (err error) {
	
	if ctx.Empty() {
		return
	}
	
	messageCount, err := ctx.Agg().CountOfBlockMessages(ctx.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}
	
	begin := ctx.Epoch()
	end := ctx.Epoch().Next()
	blockHeaders, err := ctx.Agg().BlockHeader(ctx.Context(), types.Filters{
		Start: &begin,
		End:   &end,
	})
	if err != nil {
		return
	}
	
	blockMessage := int64(0)
	for _, v := range blockHeaders {
		blockMessage += v.MessageCount
	}
	
	blocks := int64(len(blockHeaders))
	// Empty() 判的是 Tipset() 非空，而 blockHeaders 来自另一个端点 blockheader；
	// 聚合器两集合索引不一致时可能拿到空列表 ⇒ 直接除会整数除零 panic，
	// 被 recover 转成 error 后该高度永久重试卡批。无区块就记 0。
	var avgBlockMessage int64
	if blocks > 0 {
		avgBlockMessage = blockMessage / blocks
	}

	item := &po.MessageCount{
		Epoch:           ctx.Epoch().Int64(),
		Message:         messageCount,
		Block:           blocks,
		AvgBlockMessage: avgBlockMessage,
	}
	
	err = b.save(ctx.Context(), item)
	if err != nil {
		return
	}
	
	return
}

func (b MessageCountTask) save(ctx context.Context, item *po.MessageCount) (err error) {
	err = b.repo.SaveMessageCounts(ctx, item)
	return
}
