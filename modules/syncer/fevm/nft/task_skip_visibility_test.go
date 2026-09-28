package nft

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	logging "github.com/gozelle/logger"
	"github.com/stretchr/testify/require"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/events"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	transferTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	badContract   = "0x00000000000000000000000000000000000000ba"
	goodContract  = "0x00000000000000000000000000000000000000cd"
	fromTopic     = "0x0000000000000000000000001111111111111111111111111111111111111111"
	toTopic       = "0x0000000000000000000000002222222222222222222222222222222222222222"
	tokenIdTopic  = "0x000000000000000000000000000000000000000000000000000000000000002a"
)

// fakeDecoder 只覆写本用例用到的两个方法：内嵌接口让其余方法保持「未实现即 panic」，
// 一旦被测代码多调了一个接口方法，测试会立刻暴露而不是静默通过。
type fakeDecoder struct {
	fevm.ABIDecoderAPI
}

func (f fakeDecoder) CallContract(abiJson []byte, contractAddress, method string, params []*fevm.ContractParam) ([]interface{}, error) {
	if contractAddress == badContract {
		// 非 ERC721（或未实现 metadata）的合约：prepareErc721Token 会把这句
		// not found / contract reverted 的报错吞成 (nil, nil)，返回空 token。
		return nil, errors.New("method not found: tokenURI")
	}
	return []interface{}{"fake-value"}, nil
}

func (f fakeDecoder) HexToBigInt(hexStr string) (*big.Int, error) {
	v := new(big.Int)
	v.SetString(strings.TrimPrefix(hexStr, "0x"), 16)
	return v, nil
}

// captureLogs 把本包日志器换成可断言的观察者（原跳过路径无日志，加日志后必须可验证）
func captureLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	prev := log
	log = &logging.Logger{SugaredLogger: *zap.New(core).Sugar()}
	t.Cleanup(func() { log = prev })
	return logs
}

func transferEvent(contract string, cid string) *events.Event {
	return &events.Event{
		XCid: cid,
		EthReceiptLog: &londobell.EthReceiptLog{
			Address:         contract,
			Topics:          []string{transferTopic, fromTopic, toTopic, tokenIdTopic},
			TransactionHash: "0x" + strings.Repeat("ab", 32),
		},
		TX: &londobell.EthTransaction{Value: "0x1", Input: "0x1234567890abcdef"},
	}
}

// TestCollectTransfersLogsSkippedERC721InsteadOfSilentContinue 覆盖「NFT 代币准备失败静默 continue」
// 这条静默丢数路径：合约取不到 ERC721 元数据时必须留 Warn（带 epoch/合约/token_id），
// 且不得影响同一批里其它事件的正常处理。
func TestCollectTransfersLogsSkippedERC721InsteadOfSilentContinue(t *testing.T) {

	logs := captureLogs(t)

	task := NFTTask{decoder: fakeDecoder{}, abi: []byte("[]")}

	es := []*events.Event{
		transferEvent(badContract, "bafyBadCid"),
		transferEvent(goodContract, "bafyGoodCid"),
	}

	transfers, err := task.collectTransfers(6259665, es)

	require.NoError(t, err)
	require.Len(t, transfers, 1, "正常合约的事件必须照旧处理（业务语义未变）")
	require.Equal(t, goodContract, transfers[0].Contract)
	require.Equal(t, "bafyGoodCid", transfers[0].Cid)

	entries := logs.All()
	require.Len(t, entries, 1, "被跳过的合约必须留下且只留下一条 Warn 日志")
	entry := entries[0]
	require.Equal(t, zapcore.WarnLevel, entry.Level)
	msg := entry.Message
	require.Contains(t, msg, "6259665", "日志必须带 epoch")
	require.Contains(t, msg, badContract, "日志必须带合约地址")
	require.Contains(t, msg, tokenIdTopic, "日志必须带 token_id")
	require.Contains(t, msg, "bafyBadCid", "日志必须带消息 cid")
}

// TestCollectTransfersStillReturnsDecoderError 钉住「不要改业务语义」：
// prepareErc721Token 真正返回错误（非 not found/contract reverted）时仍要中断并上抛。
func TestCollectTransfersStillReturnsDecoderError(t *testing.T) {

	task := NFTTask{decoder: &fatalDecoder{}, abi: []byte("[]")}

	_, err := task.collectTransfers(6259665, []*events.Event{transferEvent(goodContract, "bafyCid")})

	require.Error(t, err, "解码器真报错时不能吞掉，保持原语义（失败即让该高度重试）")
	require.Contains(t, err.Error(), "boom")
}

// fatalDecoder 返回一个不命中 prepareErc721Token 兜底的错误 ⇒ 错误必须被上抛
type fatalDecoder struct {
	fevm.ABIDecoderAPI
}

func (f *fatalDecoder) CallContract(abiJson []byte, contractAddress, method string, params []*fevm.ContractParam) ([]interface{}, error) {
	return nil, errors.New("boom: abi decoder unavailable")
}
