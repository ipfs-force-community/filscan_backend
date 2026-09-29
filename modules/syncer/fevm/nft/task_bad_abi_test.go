package nft

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/events"
	"go.uber.org/zap/zapcore"
)

const (
	// malformedAbiContract 对部分只读方法返回「零字节 / 长度非 32 字节对齐」的脏数据，
	// 复现实测的生产报错：abi: improperly formatted output: "\x00\x00\x00..."
	malformedAbiContract = "0x00000000000000000000000000000000000000ee"
	// infraBrokenContract 模拟解码服务本身不可用（RPC 挂掉），这类错误必须照旧上抛。
	infraBrokenContract = "0x00000000000000000000000000000000000000ff"
)

// errImproperlyFormatted 与 go-ethereum accounts/abi 实际产出的文案逐字一致。
var errImproperlyFormatted = errors.New("abi: improperly formatted output: \"\\x00\\x00\\x00\" - Bytes: [0 0 0]")

type scriptedResult struct {
	res []interface{}
	err error
}

func scriptKey(contract, method string) string {
	return contract + "#" + method
}

// scriptedDecoder 按 contract+method 返回预置结果：精确复现「某合约的某只读字段被
// 畸形返回值搞崩」的场景，其余方法一律返回可用的假值。
type scriptedDecoder struct {
	fevm.ABIDecoderAPI
	results map[string]scriptedResult
}

func (d scriptedDecoder) CallContract(abiJson []byte, contractAddress, method string, params []*fevm.ContractParam) ([]interface{}, error) {
	if r, ok := d.results[scriptKey(contractAddress, method)]; ok {
		return r.res, r.err
	}
	return []interface{}{fmt.Sprintf("ok-%s", method)}, nil
}

func (d scriptedDecoder) HexToBigInt(hexStr string) (*big.Int, error) {
	v := new(big.Int)
	v.SetString(strings.TrimPrefix(hexStr, "0x"), 16)
	return v, nil
}

// TestIsMalformedMetadataOutputClassification 钉住容错的三分类边界：
//   - 合约返回畸形数据（abi: 前缀）⇒ 字段级降级
//   - 合约不是 ERC721（not found / contract reverted）⇒ 不属于字段级容错（走整条跳过）
//   - 其它（解密/RPC/解码服务不可用）⇒ 必须上抛，保持「失败即重试」语义
func TestIsMalformedMetadataOutputClassification(t *testing.T) {

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"go-ethereum 畸形输出", errImproperlyFormatted, true},
		{"RPC 透传的 abi 解码错误", errors.New("rpc error: code = Unknown desc = abi: cannot marshal in to go type: length insufficient 0 require 32"), true},
		{"方法不存在（不是 ERC721）", errors.New("method not found: tokenURI"), false},
		{"合约已 revert", errors.New("contract reverted"), false},
		{"解码服务不可用", errors.New("boom: abi decoder unavailable"), false},
		{"网络错误", errors.New("dial tcp 127.0.0.1:8545: connect: connection refused"), false},
		{"nil", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, isMalformedMetadataOutput(c.err), "isMalformedMetadataOutput(%v)", c.err)
			// isNotErc721 的口径必须与改造前的内联判断逐字一致
			require.False(t, isNotErc721(nil))
		})
	}
}

// TestPrepareErc721TokenToleratesMalformedAbiOutput 核心用例：单个只读字段解不出来时，
// 只打 WARN + 该字段留空，token 照常产出（⇒ 该高度任务不会失败、不会无限重试）。
func TestPrepareErc721TokenToleratesMalformedAbiOutput(t *testing.T) {

	logs := captureLogs(t)

	dec := scriptedDecoder{results: map[string]scriptedResult{
		scriptKey(malformedAbiContract, "symbol"): {err: errImproperlyFormatted},
	}}

	token, err := prepareErc721Token(dec, []byte("[]"), malformedAbiContract, tokenIdTopic)

	require.NoError(t, err, "单个字段畸形不得让 token 准备失败（否则整个高度任务失败）")
	require.NotNil(t, token, "token 必须照常产出，转账事件不能被丢掉")
	require.Equal(t, "", token.Symbol, "解不出的字段降级为空值")
	require.Equal(t, "ok-name", token.Name, "同契约其它字段照常取到")
	require.Equal(t, "ok-tokenURI", token.TokenUri)
	require.Equal(t, "ok-ownerOf", token.Owner)
	require.Equal(t, tokenIdTopic, token.TokenId)
	require.Equal(t, malformedAbiContract, token.Contract)

	entries := logs.All()
	require.Len(t, entries, 1, "降级必须留痕：恰好一条 Warn")
	entry := entries[0]
	require.Equal(t, zapcore.WarnLevel, entry.Level)
	require.Contains(t, entry.Message, malformedAbiContract, "Warn 必须带合约地址")
	require.Contains(t, entry.Message, "symbol", "Warn 必须带方法名")
	require.Contains(t, entry.Message, tokenIdTopic, "Warn 必须带 token_id")
	require.Contains(t, entry.Message, "improperly formatted output", "Warn 必须带原始错误，便于事后核对")
}

// TestPrepareErc721TokenToleratesNonStringAndEmptyResults 覆盖另外两类「取不到字段」：
// 返回值类型与 ABI 声明不符（本地解码器把 ownerOf 解成 address）与空返回。
func TestPrepareErc721TokenToleratesNonStringAndEmptyResults(t *testing.T) {

	logs := captureLogs(t)

	dec := scriptedDecoder{results: map[string]scriptedResult{
		scriptKey(malformedAbiContract, "ownerOf"):  {res: []interface{}{uint64(1)}}, // 非 string
		scriptKey(malformedAbiContract, "tokenURI"): {res: []interface{}{}},          // 空返回
	}}

	token, err := prepareErc721Token(dec, []byte("[]"), malformedAbiContract, tokenIdTopic)

	require.NoError(t, err)
	require.NotNil(t, token)
	require.Equal(t, "", token.Owner)
	require.Equal(t, "", token.TokenUri)
	require.Equal(t, "ok-name", token.Name)
	require.Equal(t, "ok-symbol", token.Symbol)

	entries := logs.All()
	require.Len(t, entries, 2, "两个取不到的字段各留一条 Warn")
	for _, entry := range entries {
		require.Equal(t, zapcore.WarnLevel, entry.Level)
		require.Contains(t, entry.Message, malformedAbiContract)
		require.Contains(t, entry.Message, tokenIdTopic)
	}
	msgs := entries[0].Message + "\n" + entries[1].Message
	require.Contains(t, msgs, "ownerOf")
	require.Contains(t, msgs, "tokenURI")
}

// TestPrepareErc721TokenStillNilForNonERC721 钉住「不要顺手改掉跳过语义」：
// 合约压根不是可解析的 ERC721（方法不存在 / 被 revert）时仍返回 (nil, nil)。
func TestPrepareErc721TokenStillNilForNonERC721(t *testing.T) {

	captureLogs(t)

	dec := scriptedDecoder{results: map[string]scriptedResult{
		scriptKey(badContract, "tokenURI"): {err: errors.New("method not found: tokenURI")},
		scriptKey(badContract, "name"):     {err: errors.New("method not found: name")},
		scriptKey(badContract, "symbol"):   {err: errors.New("method not found: symbol")},
		scriptKey(badContract, "ownerOf"):  {err: errors.New("contract reverted")},
	}}

	token, err := prepareErc721Token(dec, []byte("[]"), badContract, tokenIdTopic)

	require.NoError(t, err)
	require.Nil(t, token, "非 ERC721 合约必须仍返回 nil token（调用方据此跳过该事件）")
}

// TestCollectTransfersToleratesMalformedMetadata 从 collectTransfers 这一层确认
// 「单个合约的脏数据不得让整个高度任务失败」：返回 err == nil，且该合约的转账
// 照常被收集，同一批里其它合约不受影响。
func TestCollectTransfersToleratesMalformedMetadata(t *testing.T) {

	logs := captureLogs(t)

	dec := scriptedDecoder{results: map[string]scriptedResult{
		scriptKey(malformedAbiContract, "symbol"): {err: errImproperlyFormatted},
		scriptKey(malformedAbiContract, "name"):   {err: errImproperlyFormatted},
	}}

	task := NFTTask{decoder: dec, abi: []byte("[]")}

	es := []*events.Event{
		transferEvent(malformedAbiContract, "bafyBadAbiCid"),
		transferEvent(goodContract, "bafyGoodCid"),
	}

	transfers, err := task.collectTransfers(6312687, es)

	require.NoError(t, err, "畸形 ABI 返回值不得让整个高度任务失败（否则离线回放会卡死在该高度）")
	require.Len(t, transfers, 2, "两个合约的转账都必须落库，脏数据合约也不例外")
	require.Equal(t, malformedAbiContract, transfers[0].Contract)
	require.Equal(t, "bafyBadAbiCid", transfers[0].Cid)
	require.Equal(t, goodContract, transfers[1].Contract)

	entries := logs.All()
	require.Len(t, entries, 2, "两个降级字段各留一条 Warn（且不产生额外的跳过 Warn）")
	for _, entry := range entries {
		require.Equal(t, zapcore.WarnLevel, entry.Level)
		require.Contains(t, entry.Message, malformedAbiContract)
	}
}

// TestCollectTransfersStillFailsOnInfraError 钉住容错的边界：解码服务真正不可用时
// 仍然要让该高度失败（可重试），不能被「字段级容错」顺手吞掉。
func TestCollectTransfersStillFailsOnInfraError(t *testing.T) {

	logs := captureLogs(t)

	dec := scriptedDecoder{results: map[string]scriptedResult{
		scriptKey(infraBrokenContract, "symbol"): {err: errors.New("boom: abi decoder unavailable")},
	}}

	task := NFTTask{decoder: dec, abi: []byte("[]")}

	_, err := task.collectTransfers(6312687, []*events.Event{transferEvent(infraBrokenContract, "bafyCid")})

	require.Error(t, err, "基础设施故障必须照旧上抛")
	require.Contains(t, err.Error(), "boom")
	require.Empty(t, logs.All(), "基础设施故障不是字段级降级，不留 Warn")
}
