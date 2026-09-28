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

func prepareErc721Token(decoder fevm.ABIDecoderAPI, abi []byte, address string, tokenId string) (token *po.NFTToken, err error) {
	
	defer func() {
		if err != nil && (strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "contract reverted")) {
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
		res, e := decoder.CallContract(abi, address, "tokenURI", []*fevm.ContractParam{
			{
				Type:  "uint256",
				Value: tokenId,
			},
		})
		if e != nil {
			return e
		}
		r, e := extractCallResult(res)
		if e != nil {
			return fmt.Errorf("get tokenURI error: %w", e)
		}
		uri = r
		return nil
	})
	
	g.Go(func() error {
		res, e := decoder.CallContract(abi, address, "name", nil)
		if e != nil {
			return e
		}
		if e != nil {
			return e
		}
		r, e := extractCallResult(res)
		if e != nil {
			return fmt.Errorf("get name error: %w", e)
		}
		name = r
		return nil
	})
	
	g.Go(func() error {
		res, e := decoder.CallContract(abi, address, "symbol", nil)
		if e != nil {
			return e
		}
		r, e := extractCallResult(res)
		if e != nil {
			return fmt.Errorf("get symbol error: %w", e)
		}
		symbol = r
		return nil
	})
	
	g.Go(func() error {
		res, e := decoder.CallContract(abi, address, "ownerOf", []*fevm.ContractParam{
			{
				Type:  "uint256",
				Value: tokenId,
			},
		})
		if e != nil {
			return e
		}
		r, e := extractCallResult(res)
		if e != nil {
			return fmt.Errorf("get owner error: %w", e)
		}
		owner = r
		return nil
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
