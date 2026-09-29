package nft

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

// TestPrepareTokensSkipsNilToken 钉住 Calc 的 nil 保护。
//
// prepareErc721Token 对「不是可解析 ERC721 的合约」返回 (nil, nil)（见 task.go
// collectTransfers 的跳过语义）；RollBack 里一直有 nil 判断，Calc 这条路径却把返回值
// 直接 append 进 tokens，随后 SaveTokens/ResolveNFTURL 会 nil 解引用 panic，
// 让该高度计算任务失败并被框架无限重试、卡死。
func TestPrepareTokensSkipsNilToken(t *testing.T) {

	logs := captureLogs(t)

	// fakeDecoder 对 badContract 的所有只读方法都报 not found ⇒ prepareErc721Token 返回 nil token。
	calc := NFTCalculator{decoder: fakeDecoder{}, abi: []byte("[]")}

	filter := map[string]*po.NFTTransfer{
		badContract + "." + tokenIdTopic: {
			Epoch: 6312687, Cid: "bafyBadToken", Contract: badContract, TokenId: tokenIdTopic,
		},
		goodContract + "." + tokenIdTopic: {
			Epoch: 6312687, Cid: "bafyGoodToken", Contract: goodContract, TokenId: tokenIdTopic,
		},
	}

	tokens, err := calc.prepareTokens(filter)

	require.NoError(t, err, "非 ERC721 合约不是错误，只是该 token 跳过")
	require.Len(t, tokens, 1, "nil token 必须被跳过，不能 append 进待落库列表")
	for i, token := range tokens {
		require.NotNil(t, token, "tokens[%d] 不得为 nil（nil 会让 SaveTokens/ResolveNFTURL panic）", i)
	}
	require.Equal(t, goodContract, tokens[0].Contract, "留下的必须是可以解析 ERC721 的那个合约")
	require.Equal(t, tokenIdTopic, tokens[0].TokenId)

	entries := logs.All()
	require.Len(t, entries, 1, "跳过必须留痕：恰好一条 Warn")
	entry := entries[0]
	require.Equal(t, zapcore.WarnLevel, entry.Level)
	require.Contains(t, entry.Message, badContract, "Warn 必须带合约地址")
	require.Contains(t, entry.Message, tokenIdTopic, "Warn 必须带 token_id")
	require.Contains(t, entry.Message, "bafyBadToken", "Warn 必须带消息 cid")
	require.Contains(t, entry.Message, "6312687", "Warn 必须带 epoch")
}

// TestPrepareTokensNilResultIsSafeForSaveTokens 反向验证保护确实生效：
// 把 prepareTokens 的结果真的交给真实 Mapper.SaveTokens（DryRun，不连库）也不会 panic；
// 原实现把 nil 混在 list 里时这里就是 nil 解引用。
func TestPrepareTokensNilResultIsSafeForSaveTokens(t *testing.T) {

	captureLogs(t)

	db, cap := dryRunDB(t)
	calc := NFTCalculator{decoder: fakeDecoder{}, abi: []byte("[]")}

	filter := map[string]*po.NFTTransfer{
		badContract + "." + tokenIdTopic: {
			Epoch: 6312687, Cid: "bafyBadToken", Contract: badContract, TokenId: tokenIdTopic,
		},
	}

	tokens, err := calc.prepareTokens(filter)
	require.NoError(t, err)
	require.Empty(t, tokens, "该批只有非 ERC721 合约 ⇒ 没有任何 token 需要落库")

	require.NotPanics(t, func() {
		require.NoError(t, NewMapper(db).SaveTokens(context.Background(), tokens))
	}, "空列表不得把 nil 解引用带进 SaveTokens")

	require.Empty(t, insertStatements(cap.All()), "没有任何 token 时不得产生插入语句")
}

// TestPrepareTokensStillPropagatesInfraError 钉住边界：解码服务真正不可用时
// （不是「合约不是 ERC721」）必须照旧上抛，让该高度可重试，不能被 nil 保护吞掉。
func TestPrepareTokensStillPropagatesInfraError(t *testing.T) {

	captureLogs(t)

	dec := scriptedDecoder{results: map[string]scriptedResult{
		scriptKey(infraBrokenContract, "symbol"): {err: errors.New("boom: abi decoder unavailable")},
	}}
	calc := NFTCalculator{decoder: dec, abi: []byte("[]")}

	_, err := calc.prepareTokens(map[string]*po.NFTTransfer{
		infraBrokenContract + "." + tokenIdTopic: {
			Epoch: 6312687, Cid: "bafyInfra", Contract: infraBrokenContract, TokenId: tokenIdTopic,
		},
	})

	require.Error(t, err, "基础设施故障必须照旧上抛（失败即重试），不能被 nil 保护吞掉")
	require.Contains(t, err.Error(), "boom")
}
