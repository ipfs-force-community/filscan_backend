package decoder

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/asm"
	"github.com/ethereum/go-ethereum/ethclient"
	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/bundle"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	_dal "gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewContractDecoder(db *gorm.DB, ethClient *ethclient.Client) *ContractDecoder {
	return &ContractDecoder{
		db:     db,
		repo:   dal.NewFEvmDal(db),
		client: NewEthClient(ethClient),
	}
}

var log = logging.NewLogger("decoder")

var _ fevm.ContractAPI = (*ContractDecoder)(nil)

type ContractDecoder struct {
	repo   repository.FEvmRepo
	db     *gorm.DB
	client *EthClient
}

func (a ContractDecoder) ChainHead() (epoch chain.Epoch, err error) {

	r, err := a.client.BlockNumber(context.Background())
	if err != nil {
		return
	}

	epoch = chain.Epoch(r)

	return
}

func (a ContractDecoder) HexToBigInt(hexStr string) (r *big.Int, err error) {

	bs := common.Hex2Bytes(strings.TrimPrefix(hexStr, "0x"))

	r = new(big.Int)
	r.SetBytes(bs)

	return
}

func (a ContractDecoder) LookupMethod(abiJson, input []byte) (name string, err error) {
	parsed, err := abi.JSON(bytes.NewReader(abiJson))
	if err != nil {
		return
	}

	m, err := parsed.MethodById(input)
	if err != nil {
		err = nil
		return //nolint
	}
	name = m.Name
	return
}

func (a ContractDecoder) DecodeMethodInput(abiJson, input []byte) (reply *fevm.DecodeMethodInputReply, err error) {

	parsed, err := abi.JSON(bytes.NewReader(abiJson))
	if err != nil {
		return
	}

	m, err := parsed.MethodById(input)
	if err != nil {
		return
	}

	reply = &fevm.DecodeMethodInputReply{
		Name:    m.Name,
		RawName: m.String(),
	}

	reply.Values, err = m.Inputs.Unpack(input[4:])
	if err != nil {
		return
	}

	return
}

func (a ContractDecoder) DecodeEventHexAddress(addr string) (r string, err error) {
	d := common.HexToAddress(strings.TrimPrefix(addr, "0x"))
	r = d.Hex()
	return
}

func (a ContractDecoder) LookupEvent(abiJson, eventHash []byte) (name string, err error) {

	parsed, err := abi.JSON(bytes.NewReader(abiJson))
	if err != nil {
		return
	}

	e, err := parsed.EventByID(common.BytesToHash(eventHash))
	if err != nil {
		err = nil
		return //nolint
	}
	name = e.Name

	return
}

func (a ContractDecoder) DecodeEventInput(abiJson, eventHash, data []byte) (reply *fevm.DecodeEventInputReply, err error) {

	parsed, err := abi.JSON(bytes.NewReader(abiJson))
	if err != nil {
		return
	}

	m, err := parsed.EventByID(common.BytesToHash(eventHash))
	if err != nil {
		return
	}

	reply = &fevm.DecodeEventInputReply{
		Name:    m.Name,
		RawName: m.String(),
	}

	reply.Values, err = m.Inputs.Unpack(data)
	if err != nil {
		return
	}

	return
}

func (a ContractDecoder) ParseABISignatures(abiJson []byte) (parsed []*fevm.ABISignature, err error) {
	tx := a.db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	ctx := _dal.ContextWithDB(context.Background(), tx)

	items, err := a.parseABISignatures(abiJson)
	if err != nil {
		return
	}

	err = a.repo.SaveAPISignatures(ctx, items)
	if err != nil {
		return
	}

	for _, v := range items {
		parsed = append(parsed, &fevm.ABISignature{
			Type: v.Type,
			Name: v.Name,
			Id:   v.Id,
			Raw:  v.Raw,
		})
	}

	return
}

func (a ContractDecoder) parseABISignatures(abiJson []byte) (items []*po.FEvmABISignature, err error) {
	abiParsed, err := abi.JSON(bytes.NewReader(abiJson))
	if err != nil {
		return
	}
	for _, v := range abiParsed.Methods {
		items = append(items, &po.FEvmABISignature{
			Type: "method",
			Id:   fmt.Sprintf("0x%s", hex.EncodeToString(v.ID)),
			Name: v.Name,
			Raw:  v.String(),
		})
	}

	for _, v := range abiParsed.Events {
		items = append(items, &po.FEvmABISignature{
			Type: "event",
			Name: v.Name,
			Id:   v.ID.Hex(),
			Raw:  v.String(),
		})
	}
	return
}

func (a ContractDecoder) parseABISignaturesMaps(abiJson []byte) (methods, events map[string]bool, err error) {

	items, err := a.parseABISignatures(abiJson)
	if err != nil {
		return
	}

	methods = map[string]bool{}
	events = map[string]bool{}
	for _, v := range items {
		switch v.Type {
		case "method":
			methods[v.Id] = false
		case "event":
			events[v.Id] = false
		}
	}

	return
}

func (a ContractDecoder) DetectContractProtocol(bytecode []byte) (protocols fevm.Protocols, err error) {
	var opItems []*OpItem

	it := asm.NewInstructionIterator(bytecode)
	for it.Next() {
		if it.Arg() != nil && 0 < len(it.Arg()) {
			if strings.ToUpper(it.Op().String()) == "PUSH4" {
				opItems = append(opItems, &OpItem{
					Type: "method",
					ID:   fmt.Sprintf("0x%x", it.Arg()),
				})
			} else if strings.ToUpper(it.Op().String()) == "PUSH32" {
				opItems = append(opItems, &OpItem{
					Type: "event",
					ID:   fmt.Sprintf("0x%x", it.Arg()),
				})
			}
		}
	}
	e := it.Error()
	_ = e

	protocols.Erc20, _ = a.IsEcr20(opItems)
	protocols.Erc721, _ = a.IsEcr721(opItems)
	protocols.Erc721MetaData, _ = a.IsEcr721MetaData(opItems)
	protocols.Pair, _ = a.IsPair(opItems)
	return
}

type OpItem struct {
	Type string
	ID   string
}

func (a ContractDecoder) IsEcr20(opItems []*OpItem) (ok bool, err error) {

	file, err := bundle.Templates.Open("/erc20.json")
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()

	c, err := io.ReadAll(file)
	if err != nil {
		return
	}

	methods, events, err := a.parseABISignaturesMaps(c)
	if err != nil {
		return
	}

	ok = a.matchProtocol(methods, events, opItems)

	return
}

func (a ContractDecoder) IsPair(opItems []*OpItem) (ok bool, err error) {

	file, err := bundle.Templates.Open("/pair.json")
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()

	c, err := io.ReadAll(file)
	if err != nil {
		return
	}

	methods, events, err := a.parseABISignaturesMaps(c)
	if err != nil {
		return
	}

	ok = a.matchProtocol(methods, events, opItems)

	return
}

func (a ContractDecoder) IsEcr721(opItems []*OpItem) (ok bool, err error) {

	file, err := bundle.Templates.Open("/erc721.json")
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()

	c, err := io.ReadAll(file)
	if err != nil {
		return
	}

	methods, events, err := a.parseABISignaturesMaps(c)
	if err != nil {
		return
	}

	ok = a.matchProtocol(methods, events, opItems)

	return
}

func (a ContractDecoder) IsEcr721MetaData(opItems []*OpItem) (ok bool, err error) {

	file, err := bundle.Templates.Open("/erc721-metadata.json")
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()

	c, err := io.ReadAll(file)
	if err != nil {
		return
	}

	methods, events, err := a.parseABISignaturesMaps(c)
	if err != nil {
		return
	}

	ok = a.matchProtocol(methods, events, opItems)

	return
}

func (a ContractDecoder) matchProtocol(methods, events map[string]bool, opItems []*OpItem) (match bool) {

	for _, v := range opItems {
		if _, ok := methods[v.ID]; ok {
			methods[v.ID] = true
		}
		if _, ok := events[v.ID]; ok {
			events[v.ID] = true
		}
	}
	match = true
	for _, v := range methods {
		if !v {
			match = false
		}
	}
	for _, v := range events {
		if !v {
			match = false
		}
	}

	return
}

func (a ContractDecoder) HexToEthAddress(hexStr string) (ethAddr string, err error) {
	ethAddr = common.HexToAddress(hexStr).String()
	return
}

func (a ContractDecoder) CallContract(abiJson []byte, contractAddress string, method string, params []*fevm.ContractParam) (result []interface{}, err error) {
	err = a.client.Call(context.Background(), abiJson, contractAddress, method, params, &result)
	if err != nil {
		log.Debugf("call contract: %s  method: %s params: %v error: %s", contractAddress, method, params, err)
		return
	}

	return
}
