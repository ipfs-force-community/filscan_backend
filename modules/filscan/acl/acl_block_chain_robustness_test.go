package acl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gozelle/mix"
	"github.com/ipfs/go-cid"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

// fakeBlockChainAgg 只实现测例走到的方法；其余方法继承 nil interface，
// 一旦被调用就 panic——这样测例不会偷偷依赖没打桩的下游。
type fakeBlockChainAgg struct {
	AggBlockChainAcl
	block     *londobell.BlockHeader
	rewards   []*londobell.MinerBlockReward
	winCounts []*londobell.MinerWinCount
	latest    int64
	traces    []*londobell.MessageTrace
	children  []*londobell.MessageTrace
	pool      *londobell.MessagePool
	poolErr   error
}

func (f *fakeBlockChainAgg) BlockHeaderByCid(ctx context.Context, id string) ([]*londobell.BlockHeader, error) {
	if f.block == nil {
		return nil, nil
	}
	return []*londobell.BlockHeader{f.block}, nil
}

func (f *fakeBlockChainAgg) MinerBlockReward(ctx context.Context, addr chain.SmartAddress, filter types.Filters) ([]*londobell.MinerBlockReward, error) {
	return f.rewards, nil
}

func (f *fakeBlockChainAgg) WinCount(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinerWinCount, error) {
	return f.winCounts, nil
}

func (f *fakeBlockChainAgg) ParentTipset(ctx context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	return nil, nil
}

func (f *fakeBlockChainAgg) LatestTipset(ctx context.Context) ([]*londobell.Tipset, error) {
	return []*londobell.Tipset{{ID: f.latest}}, nil
}

func (f *fakeBlockChainAgg) ChildTransfersForMessage(ctx context.Context, id string) ([]*londobell.MessageTrace, error) {
	return f.children, nil
}

func (f *fakeBlockChainAgg) TraceForMessage(ctx context.Context, id string) ([]*londobell.MessageTrace, error) {
	return f.traces, nil
}

func (f *fakeBlockChainAgg) MessagePool(ctx context.Context, id string, filters *types.Filters) (*londobell.MessagePool, error) {
	return f.pool, f.poolErr
}

func (f *fakeBlockChainAgg) BlocksForMessage(ctx context.Context, id string) ([]*londobell.BlockHeader, error) {
	return nil, nil
}

func (f *fakeBlockChainAgg) Tipset(ctx context.Context, epoch chain.Epoch) ([]*londobell.Tipset, error) {
	return nil, nil
}

func (f *fakeBlockChainAgg) HashByMessageCid(ctx context.Context, messageCid string) (*londobell.CidOrHash, error) {
	return nil, nil
}

// TestGetBlockDetails_PicksWinCountOfBlockMiner 复现线上缺陷：
// /aggregators/wincount 是「一个 epoch 每个矿工一行」，旧实现直接取 [0]，
// 会把别的矿工的出块数/手续费奖励显示到当前块上。
func TestGetBlockDetails_PicksWinCountOfBlockMiner(t *testing.T) {
	agg := &fakeBlockChainAgg{
		// 链头 100，块在 100：lag=0，不走「还没算出来」的兜底
		latest: 100,
		block:  &londobell.BlockHeader{ID: "bafyblock", Epoch: 100, Miner: "01001"},
		rewards: []*londobell.MinerBlockReward{
			{Id: 1, TotalBlockReward: decimal.NewFromInt(100), BlockCount: 1},
		},
		winCounts: []*londobell.MinerWinCount{
			{Id: "01002", TotalWinCount: 7, TotalGasReward: decimal.NewFromInt(70)},
			{Id: "01001", TotalWinCount: 3, TotalGasReward: decimal.NewFromInt(11)},
		},
	}
	b := BlockChainAclImpl{agg: agg}

	details, err := b.GetBlockDetails(context.Background(), "bafyblock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details.WinCount != 3 {
		t.Fatalf("win count = %d, want 3（必须是本矿工 01001 的，而不是列表第一条）", details.WinCount)
	}
	if details.BlockBasic.TxFeeReward == nil || !details.BlockBasic.TxFeeReward.Equal(decimal.NewFromInt(11)) {
		t.Fatalf("tx fee reward = %v, want 11", details.BlockBasic.TxFeeReward)
	}
}

// TestGetBlockDetails_HeadBlockMissingRewardIsZeroFilled 链头新块的奖励还没算出来时，
// 不能整页 500（线上每天 ~1200 次，主力是爬虫），应按空值渲染。
func TestGetBlockDetails_HeadBlockMissingRewardIsZeroFilled(t *testing.T) {
	agg := &fakeBlockChainAgg{
		latest: 102,
		block:  &londobell.BlockHeader{ID: "bafyhead", Epoch: 100, Miner: "01001"},
	}
	b := BlockChainAclImpl{agg: agg}

	details, err := b.GetBlockDetails(context.Background(), "bafyhead")
	if err != nil {
		t.Fatalf("链头块缺奖励不应报错，得到: %v", err)
	}
	if details == nil {
		t.Fatal("details 为 nil")
	}
	if !details.BlockBasic.Reward.IsZero() || details.WinCount != 0 {
		t.Fatalf("期望奖励/出块数为 0，得到 reward=%s winCount=%d", details.BlockBasic.Reward, details.WinCount)
	}
}

// TestGetBlockDetails_OldBlockWithoutRewardIsNotFound 老块没有奖励行 ⇒ 基本是孤块/回滚块：
// 按 not found 回 4xx（爬虫会停止重试），不能再记 500。
func TestGetBlockDetails_OldBlockWithoutRewardIsNotFound(t *testing.T) {
	agg := &fakeBlockChainAgg{
		latest: 5000,
		block:  &londobell.BlockHeader{ID: "bafyold", Epoch: 100, Miner: "01001"},
	}
	b := BlockChainAclImpl{agg: agg}

	details, err := b.GetBlockDetails(context.Background(), "bafyold")
	if err == nil || details != nil {
		t.Fatalf("老块无奖励行应返回 not found，得到 details=%+v err=%v", details, err)
	}
	var warn *mix.Warn
	if !errors.As(err, &warn) {
		t.Fatalf("期望客户端错误类型 *mix.Warn，得到 %T: %v", err, err)
	}
	if warn.Code != 404 {
		t.Fatalf("错误码 = %d, want 404", warn.Code)
	}
	if !strings.Contains(warn.Message, "block not found") {
		t.Fatalf("错误信息不对: %s", warn.Message)
	}
}

func mustCid(t *testing.T, s string) cid.Cid {
	t.Helper()
	c, err := cid.Decode(s)
	if err != nil {
		t.Fatalf("decode cid %s: %v", s, err)
	}
	return c
}

const (
	testCidRequested = "bafy2bzaceamegfnmphfdidxfftn6chixofbqca3ra3rqwbjvdvgrtktmgf5lu"
	testCidOther     = "bafy2bzacebesvkowodysx7blqlsr7wbin6njpbnp4yabzez3y4zgn5lfed6sg"
)

// TestGetMessageDetails_RejectsPendingMessageWithOtherCid 复现线上缺陷：
// 聚合器的 mpool 查询不认 cid，随便什么 cid 都会拿到最新一条 pending 消息。
// 旧实现直接把它当结果返回 ⇒ 任意（甚至伪造）cid 都能看到别人的消息。
func TestGetMessageDetails_RejectsPendingMessageWithOtherCid(t *testing.T) {
	agg := &fakeBlockChainAgg{
		pool: &londobell.MessagePool{
			TotalCount: 1,
			PendingMessage: []*londobell.PendingMessage{
				{Cid: mustCid(t, testCidOther), Hash: "0xdead", From: "f1abc", To: "f1def"},
			},
		},
	}
	b := BlockChainAclImpl{agg: agg}

	details, err := b.GetMessageDetails(context.Background(), testCidRequested)
	if err == nil {
		t.Fatalf("cid 不匹配时必须报错，实际返回了消息: %+v", details)
	}
	if details != nil {
		t.Fatalf("cid 不匹配时不能返回任何消息内容: %+v", details)
	}
	if !strings.Contains(err.Error(), "message not found") {
		t.Fatalf("错误信息不对: %v", err)
	}
}

// TestGetMessageDetails_PoolLookupFailureIsNotFound mpool 解析失败（pending 消息字段还是空串）
// 属于「查不到」，不能把 500 抛给调用方。
func TestGetMessageDetails_PoolLookupFailureIsNotFound(t *testing.T) {
	agg := &fakeBlockChainAgg{poolErr: errTestPoolDecode}
	b := BlockChainAclImpl{agg: agg}

	details, err := b.GetMessageDetails(context.Background(), testCidRequested)
	if err == nil || details != nil {
		t.Fatalf("pool 解析失败应返回 not found，得到 details=%+v err=%v", details, err)
	}
	if !strings.Contains(err.Error(), "message not found") {
		t.Fatalf("错误信息不对: %v", err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

const errTestPoolDecode = testError("unmarshal error:error decoding string '\"\"': can't convert \"\" to decimal")
