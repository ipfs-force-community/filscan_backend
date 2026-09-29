package nft

import (
	"context"
	"fmt"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/async/parallel"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/bundle"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/events"
	"gorm.io/gorm"
	"io"
	"strings"
)

// log 本包的日志器（原有跳过路径无任何日志，属静默丢数；见 collectTransfers）
var log = logging.NewLogger("nft")

func NewNFTTask(db *gorm.DB, decoder fevm.ABIDecoderAPI) *NFTTask {
	
	return &NFTTask{
		mapper:  NewMapper(db),
		decoder: decoder,
		abi:     initErc721MetadataAbi(),
	}
}

func initErc721MetadataAbi() []byte {
	file, err := bundle.Templates.Open("/erc721-metadata.json")
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = file.Close()
	}()
	
	c, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}
	return c
}

var _ syncer.Task = (*NFTTask)(nil)

type NFTTask struct {
	mapper  iMapper
	decoder fevm.ABIDecoderAPI
	abi     []byte
}

func (e NFTTask) Name() string {
	return "nft-task"
}

func (e NFTTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	
	err = e.mapper.DeleteTransfersAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}
	
	return
}

func (e NFTTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (e NFTTask) Exec(ctx *syncer.Context) (err error) {
	
	if ctx.Empty() {
		return
	}
	
	r, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := r.([]*londobell.TraceMessage)
	
	n := events.NewEvents(ctx.Agg(), traces)
	es, err := n.GetEvents(ctx.Context())
	if err != nil {
		err = fmt.Errorf("parse events error: %s", err)
		return
	}
	
	var transfers []*po.NFTTransfer

	transfers, err = e.collectTransfers(ctx.Epoch().Int64(), es)
	if err != nil {
		return
	}

	err = e.save(ctx.Context(), transfers)
	if err != nil {
		return
	}

	return
}

// collectTransfers 把一批 ERC721 transfer 事件整理成待入库记录。
//
// 抽出来只为可测（循环体与原来逐字一致，唯一变化是下面那条 Warn 日志）：原实现里
// prepareErc721Token 返回 (nil, nil)（合约不是可解析的 ERC721，decoder 报 not found /
// contract reverted）时直接 continue —— 无日志、无台账、指针照走，属于静默丢数路径，
// 事后无法从日志定位「哪些合约的哪些 token 被丢了」。这里补上带 epoch/合约/token_id
// 的 Warn 留痕，业务语义不变（仍然跳过该事件，不影响本高度其余事件）。
func (e NFTTask) collectTransfers(epoch int64, es []*events.Event) (transfers []*po.NFTTransfer, err error) {

	for _, v := range es {
		// 处理所有 erc721-metadata transfer 事件

		if v.Topics[0] != "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef" {
			continue
		}

		if len(v.Topics) != 4 {
			continue
		}

		// 检查合约类型是否为 erc721-metadata,
		var token *po.NFTToken
		token, err = prepareErc721Token(e.decoder, e.abi, v.Address, v.Topics[3])
		if err != nil {
			return
		}
		if token == nil {
			// 该合约取不到 ERC721 元数据（不是 ERC721 / 合约已自毁 / 未实现 metadata）。
			// 跳过是正确的，但不能静默：留一条带 epoch、合约、token_id、消息 cid 的 Warn，
			// 便于事后核对「某高度某合约某 token 的转账是否真的丢了」。
			log.Warnf("epoch %d 跳过 NFT transfer 事件：合约取不到 ERC721 元数据（token 准备为空，非可解析 ERC721）: contract=%s token_id=%s cid=%s tx=%s",
				epoch, v.Address, v.Topics[3], v.XCid, v.TransactionHash)
			continue
		}

		var transfer *po.NFTTransfer
		transfer, err = e.prepareErc721Transfer(epoch, v)
		if err != nil {
			return
		}

		transfers = append(transfers, transfer)
	}

	return
}

func (e NFTTask) save(ctx context.Context, transfers []*po.NFTTransfer) (err error) {
	
	if len(transfers) > 0 {
		err = e.mapper.SaveTransfers(ctx, transfers)
		if err != nil {
			return
		}
	}
	
	return
}

func (e NFTTask) prepareErc721Transfer(epoch int64, event *events.Event) (transfer *po.NFTTransfer, err error) {
	item, err := e.decoder.HexToBigInt(event.Topics[3])
	if err != nil {
		return
	}
	b, err := e.decoder.HexToBigInt(event.TX.Value)
	if err != nil {
		return
	}
	
	var method string
	if event.TX != nil && len(event.TX.Input) >= 10 {
		method = event.TX.Input[:10]
	}
	
	transfer = &po.NFTTransfer{
		Epoch:    epoch,
		Cid:      event.XCid,
		Contract: event.Address,
		From:     chain.TrimHexAddress(event.Topics[1]),
		To:       chain.TrimHexAddress(event.Topics[2]),
		TokenId:  event.Topics[3],
		Item:     item.String(),
		Method:   method,
		Value:    decimal.NewFromBigInt(b, 0),
	}
	return
}

func extractCallResult(res []interface{}) (string, error) {
	if len(res) == 0 {
		return "", fmt.Errorf("result is empty")
	}
	s, ok := res[0].(string)
	if !ok {
		return "", fmt.Errorf("res[0] is not string")
	}
	return s, nil
}

func callContractCollection(decoder fevm.ABIDecoderAPI, abi []byte, address string, tokenId string) (name string, err error) {
	res, err := decoder.CallContract(abi, address, "symbol", nil)
	if err != nil {
		return
	}
	r, e := extractCallResult(res)
	if e != nil {
		err = fmt.Errorf("get symbol error: %w", e)
		return
	}
	name = r
	return
}

// isNotErc721 判定「该合约不是可解析的 ERC721」：方法不存在 / 调用被 revert。
// 口径与改造前 prepareErc721Token 的 defer 内联判断逐字一致；这类合约的 transfer
// 事件应当整条跳过（token 返回 nil），不属于字段级容错范围。
func isNotErc721(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "contract reverted")
}

// isMalformedMetadataOutput 判定「合约返回了数据，但按 ABI 解不出该只读字段」。
//
// 实测触发场景：合约对 symbol / name / tokenURI 返回零字节或长度非 32 字节对齐的
// 脏数据，go-ethereum 解包时报 `abi: improperly formatted output: ...`。
// 这类失败完全由合约自身的返回值决定，重试永远不会自愈 ⇒ 必须降级（WARN + 空值），
// 否则单个合约就会让整个高度任务反复失败，离线回放会卡在该高度无限重试。
//
// 判定刻意排在 isNotErc721 之后：not found / contract reverted 走「跳过整条事件」的
// 既有语义；其余错误（RPC / 解码服务不可用等）保持上抛，让该高度照常重试。
func isMalformedMetadataOutput(err error) bool {
	if err == nil || isNotErc721(err) {
		return false
	}
	// go-ethereum accounts/abi 的解码/解包错误统一以 "abi: " 开头，
	// 例如 "abi: improperly formatted output: ..."、"abi: cannot unmarshal ..."。
	return strings.Contains(err.Error(), "abi: ")
}

// fetchMetadataField 取合约的一个只读元数据字段（按 ABI 解成 string）。
//
// 字段级容错契约：
//   - 取到值 ⇒ (值, nil)
//   - 合约返回值解不出该字段（ABI 解码失败 / 返回类型与 ABI 声明不符 / 空返回）
//     ⇒ ("", nil) 并打一条 WARN 留痕，降级继续 —— 单个合约的脏数据不得让整个高度失败
//   - 其它错误（RPC / 解码服务不可用等）⇒ ("", err) 上抛，保持「失败即重试」原语义
func fetchMetadataField(decoder fevm.ABIDecoderAPI, abiJson []byte, address, method, tokenId string, params []*fevm.ContractParam) (value string, err error) {

	res, err := decoder.CallContract(abiJson, address, method, params)
	if err != nil {
		if !isMalformedMetadataOutput(err) {
			return "", err
		}
		log.Warnf("合约 %s 方法 %s 的返回值畸形、无法按 ABI 解码（token_id=%s），该字段降级为空值继续，不让整个高度失败: %s",
			address, method, tokenId, err)
		return "", nil
	}

	value, err = extractCallResult(res)
	if err != nil {
		// 解码器本身没报错，但首个返回值不能当字符串用（如 ownerOf 解成 address / 返回为空）：
		// 同属「取不到该字段」，同样降级为空值而不是让整个高度失败。
		log.Warnf("合约 %s 方法 %s 的返回值无法当作字符串使用（token_id=%s），该字段降级为空值继续，不让整个高度失败: %s",
			address, method, tokenId, err)
		return "", nil
	}

	return value, nil
}

func prepareErc721Token(decoder fevm.ABIDecoderAPI, abi []byte, address string, tokenId string) (token *po.NFTToken, err error) {
	
	defer func() {
		// 合约不是可解析的 ERC721（方法不存在 / 调用被 revert）⇒ 整条事件跳过（token 返回 nil）。
		// 口径与改造前逐字一致，只是把内联判定抽成具名函数以便复用。
		if isNotErc721(err) {
			err = nil
		}
	}()
	
	var (
		uri    string
		name   string
		symbol string
		owner  string
	)
	
	g := parallel.NewGroup()
	
	g.Go(func() error {
		var e error
		uri, e = fetchMetadataField(decoder, abi, address, "tokenURI", tokenId, []*fevm.ContractParam{
			{
				Type:  "uint256",
				Value: tokenId,
			},
		})
		return e
	})
	
	g.Go(func() error {
		var e error
		name, e = fetchMetadataField(decoder, abi, address, "name", tokenId, nil)
		return e
	})
	
	g.Go(func() error {
		var e error
		symbol, e = fetchMetadataField(decoder, abi, address, "symbol", tokenId, nil)
		return e
	})
	
	g.Go(func() error {
		var e error
		owner, e = fetchMetadataField(decoder, abi, address, "ownerOf", tokenId, []*fevm.ContractParam{
			{
				Type:  "uint256",
				Value: tokenId,
			},
		})
		return e
	})
	
	err = g.Wait()
	if err != nil {
		return
	}
	
	item, err := decoder.HexToBigInt(tokenId)
	if err != nil {
		return
	}
	
	token = &po.NFTToken{
		TokenId:  tokenId,
		Contract: address,
		Name:     name,
		Symbol:   symbol,
		TokenUri: uri,
		TokenUrl: nil,
		Owner:    owner,
		Item:     item.String(),
	}
	
	return
}
