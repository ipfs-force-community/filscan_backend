package dal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

type ERC20Dal struct {
	*_dal.BaseDal
}

func (e ERC20Dal) GetERC20TransferTokenNamesByRelatedAddr(ctx context.Context, addr string) ([]string, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{}
	err = tx.Model(&po.FEvmErc20Transfer{}).Where("\"from\" = ? or \"to\" = ?", addr, addr).Distinct().Pluck("token_name", &out).Error
	return out, err
}

func (e ERC20Dal) GetDexInfo(ctx context.Context, contractID string) (*po.DexInfo, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := po.DexInfo{}

	err = tx.First(&res, "contract_id = ?", contractID).Error
	return &res, err
}

func (e ERC20Dal) GetAllMethodsDecodeSignature(ctx context.Context) ([]po.FEvmMethods, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}

	res := []po.FEvmMethods{}
	err = tx.Find(&res).Error
	return res, err
}

func (e ERC20Dal) GetContractsUrl(ctx context.Context, contracts []string) ([]*po.ContractIcons, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := []*po.ContractIcons{}
	err = tx.Find(&res, "contract_id in (?)", contracts).Error
	return res, err
}

func (e ERC20Dal) GetUniqueContractsInTransfers(ctx context.Context) ([]string, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}

	out := []string{}
	err = tx.Model(&po.FEvmErc20Transfer{}).Distinct().Pluck("contract_id", &out).Error
	return out, err
}

func (e ERC20Dal) GetAllERC20FreshContracts(ctx context.Context) ([]*po.FEvmErc20FreshList, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}

	res := []*po.FEvmErc20FreshList{}
	err = tx.Find(&res).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
	}
	return res, err
}

func (e ERC20Dal) GetERC20TransferByRelatedAddr(ctx context.Context, addr, tokenName string, page, limit int) (int64, []*po.FEvmERC20Transfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, nil, err
	}

	total := new(int64)
	out := []*po.FEvmERC20Transfer{}
	if tokenName == "" || tokenName == "all" {
		err = tx.Find(&po.FEvmERC20Transfer{}, "\"from\" = ? or \"to\" = ?", addr, addr).Count(total).Error
		if err != nil {
			return 0, nil, err
		}
		err = tx.Offset(limit*page).Limit(limit).Order("epoch desc").Find(&out, "\"from\" = ? or \"to\" = ?", addr, addr).Error
	} else {
		err = tx.Find(&po.FEvmERC20Transfer{}, "(\"from\" = ? or \"to\" = ?) and token_name = ?", addr, addr, tokenName).Count(total).Error
		if err != nil {
			return 0, nil, err
		}
		err = tx.Offset(limit*page).Limit(limit).Order("epoch desc").Find(&out, "(\"from\" = ? or \"to\" = ?) and token_name = ?", addr, addr, tokenName).Error
	}

	return *total, out, err
}

func (e ERC20Dal) GetEvmEventSignatures(ctx context.Context, hexSignature []string) (signature []*po.EvmEventSignature, err error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where("hex_signature in ?", hexSignature).Find(&signature).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			signature = nil
		}
	}

	return
}

func (e ERC20Dal) GetMethodsDecodeSignature(ctx context.Context, hex string) (string, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return "", err
	}
	res := []po.FEvmMethods{}
	err = tx.Model(&po.FEvmMethods{}).Order("id asc").Find(&res, "hex_signature = ?", hex).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		fmt.Println("find in fevm methods failed", err)
	}
	if len(res) == 0 {
		return "", nil
	}
	return res[0].Decode, err
}

func (e ERC20Dal) GetERC20AmountOfOneAddress(ctx context.Context, address string) ([]*po.FEvmERC20Balance, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := []*po.FEvmERC20Balance{}
	err = tx.Find(&res, "owner = ?", address).Error
	return res, err
}

func (e ERC20Dal) CleanERC20TransferBatch(ctx context.Context, epoch int) error {
	tx, err := e.DB(ctx)
	if err != nil {
		return err
	}
	return tx.Delete(&po.FEvmErc20Transfer{}, "epoch >= ?", epoch).Error
}

func (e ERC20Dal) CleanERC20SwapInfo(ctx context.Context, epoch int) error {
	tx, err := e.DB(ctx)
	if err != nil {
		return err
	}
	return tx.Delete(&po.FEvmERC20SwapInfo{}, "epoch >= ?", epoch).Error
}

func (e ERC20Dal) GetERC20TransferBatchAfterEpoch(ctx context.Context, epoch int) ([]*po.FEvmErc20Transfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}

	ft := []*po.FEvmErc20Transfer{}
	err = tx.Find(&ft, "epoch >= ?", epoch).Error
	if err != nil {
		return nil, err
	}
	return ft, nil
}

func (e ERC20Dal) GetERC20TransferBatchAfterEpochInOneContract(ctx context.Context, contractId string, epoch, limit, page int) (int64, []*po.FEvmERC20Transfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, nil, err
	}

	ft := []*po.FEvmERC20Transfer{}
	total := new(int64)
	err = tx.Find(&po.FEvmERC20Transfer{}, "epoch >= ? and contract_id = ?", epoch, contractId).Count(total).Error
	if err != nil {
		return 0, nil, err
	}
	err = tx.Offset((limit-1)*page).Limit(limit).Order("epoch desc").Find(&ft, "epoch >= ? and contract_id = ?", epoch, contractId).Error
	if err != nil {
		return 0, nil, err
	}
	return *total, ft, nil
}

func (e ERC20Dal) GetERC20SwapInfoByContract(ctx context.Context, contractID string, page, limit int) (int64, []*po.FEvmERC20SwapInfo, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, nil, err
	}

	total := new(int64)
	err = tx.Find(&po.FEvmERC20SwapInfo{}, "amount_in_contract_id = ? or amount_out_contract_id = ?", contractID, contractID).Count(total).Error
	if err != nil {
		return 0, nil, err
	}

	out := []*po.FEvmERC20SwapInfo{}
	err = tx.Offset(limit*page).Limit(limit).Order("epoch desc").Find(&out, "amount_in_contract_id = ? or amount_out_contract_id = ?", contractID, contractID).Error
	return *total, out, err
}

func (e ERC20Dal) GetERC20SwapInfoByCid(ctx context.Context, cid string) (*po.FEvmERC20SwapInfo, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	out := []*po.FEvmERC20SwapInfo{}
	err = tx.Find(&out, "cid = ?", cid).Error
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, nil
	}
	return out[0], nil
}

func (e ERC20Dal) GetAllERC20Contracts(ctx context.Context) ([]*po.FEvmERC20Contract, error) {
	res := []*po.FEvmERC20Contract{}
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	err = tx.Find(&res).Error
	return res, err
}

func (e ERC20Dal) UpdateOneERC20Contract(ctx context.Context, contractID string, contract *po.FEvmERC20Contract) error {
	tx, err := e.DB(ctx)
	if err != nil {
		return err
	}

	return tx.Model(&po.FEvmERC20Contract{}).Where("contract_id = ?", contractID).
		Update("total_supply", contract.TotalSupply).Error
}

func (e ERC20Dal) GetOneERC20Contract(ctx context.Context, contractID string) (*po.FEvmERC20Contract, error) {
	res := &po.FEvmERC20Contract{}
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}
	err = tx.First(&res, "contract_id = ?", contractID).Error
	return res, err
}

func (e ERC20Dal) GetUniqueTokenHolderByContract(ctx context.Context, contractID string) (int64, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, err
	}

	res := int64(0)
	err = tx.Model(&po.FEvmERC20Balance{}).Where("contract_id = ?", contractID).Count(&res).Error
	return res, err
}

func (e ERC20Dal) GetUniqueNoneZeroTokenHolderByContract(ctx context.Context, contractID string) (int64, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, err
	}

	res := int64(0)
	err = tx.Model(&po.FEvmERC20Balance{}).Where("contract_id = ? and amount != 0 ", contractID).Count(&res).Error
	return res, err
}

func UniqueERC20BalanceData(items []*po.FEvmERC20Balance) ([]*po.FEvmERC20Balance, error) {
	mp := map[string]decimal.Decimal{}
	for i := range items {
		key := fmt.Sprintf("%s-%s", items[i].Owner, items[i].ContractId)
		mp[key] = items[i].Amount
	}

	res := []*po.FEvmERC20Balance{}
	for i := range mp {
		s := strings.Split(i, "-")
		if len(s) != 2 {
			return nil, fmt.Errorf("bad eth address")
		}
		item := &po.FEvmERC20Balance{
			Owner:      s[0],
			ContractId: s[1],
			Amount:     mp[i],
		}

		res = append(res, item)
	}
	return res, nil
}

func (e ERC20Dal) UpsertERC20BalanceBatch(ctx context.Context, items []*po.FEvmERC20Balance) (err error) {
	uni, err := UniqueERC20BalanceData(items)
	if err != nil {
		return err
	}
	tx, err := e.DB(ctx)
	if err != nil {
		return err
	}

	if len(uni) == 0 {
		return nil
	}

	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "owner"},
			{Name: "contract_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"amount"}),
	}).Create(&uni).Error
}

func (e ERC20Dal) GetERC20BalanceByContract(ctx context.Context, contractID, filter string, page, limit int) (int64, []*po.FEvmERC20Balance, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, nil, err
	}

	total := new(int64)
	out := []*po.FEvmERC20Balance{}

	if filter == "" || filter == "all" {
		err = tx.Find(&po.FEvmERC20Balance{}, "contract_id = ?", contractID).Count(total).Error
		if err != nil {
			return 0, nil, err
		}

		err = tx.Offset(limit*page).Limit(limit).Order("amount desc").Find(&out, "contract_id = ?", contractID).Error
	} else {
		conds := "contract_id = ? and amount " + filter
		err = tx.Find(&po.FEvmERC20Balance{}, conds, contractID).Count(total).Error
		if err != nil {
			return 0, nil, err
		}

		err = tx.Offset(limit*page).Limit(limit).Order("amount desc").Find(&out, conds, contractID).Error

	}
	return *total, out, err
}

func (e ERC20Dal) GetERC20TransferByContract(ctx context.Context, contractID string, page, limit int) (int64, []*po.FEvmERC20Transfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, nil, err
	}

	total := new(int64)
	err = tx.Find(&po.FEvmERC20Transfer{}, "contract_id = ?", contractID).Count(total).Error
	if err != nil {
		return 0, nil, err
	}
	out := []*po.FEvmERC20Transfer{}
	err = tx.Offset(limit*page).Limit(limit).Order("epoch desc").Find(&out, "contract_id = ?", contractID).Error
	return *total, out, err
}

func (e ERC20Dal) GetERC20TransferInDexByContract(ctx context.Context, contractID string, page, limit int) (int64, []*po.FEvmERC20Transfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return 0, nil, err
	}

	total := new(int64)
	err = tx.Find(&po.FEvmERC20Transfer{}, "contract_id = ? and dex != ?", contractID, contractID).Count(total).Error
	if err != nil {
		return 0, nil, err
	}

	out := []*po.FEvmERC20Transfer{}
	err = tx.Offset(limit*page).Limit(limit).Order("epoch desc").Find(&out, "contract_id = ? and dex != ?", contractID).Error
	return *total, out, err
}

func (e ERC20Dal) GetERC20TransferInMessage(ctx context.Context, cid string) ([]*po.FEvmERC20Transfer, error) {
	tx, err := e.DB(ctx)
	if err != nil {
		return nil, err
	}

	out := []*po.FEvmERC20Transfer{}
	err = tx.Find(&out, "cid = ?", cid).Error
	return out, err
}

// FilterExistingERC20Transfers 是 erc20 转账「先查后跳」守卫里不依赖数据库的纯计算部分：
// 从 items 中剔除 (epoch, cid, index) 三元组已经存在于 existing（从库里查出来的已有行）的项，
// 其余按原顺序保留。
//
// 为什么幂等只能放应用层：fevm.erc_20_transfers 上有 INSERT 规则
// （range_insert_action_rule ⇒ fevm.action_erc_20_transfers_range_insert），
// PG 对带 INSERT/UPDATE 规则的表直接禁止 ON CONFLICT
// （SQLSTATE 0A000: INSERT with ON CONFLICT clause cannot be used with table that has
// INSERT or UPDATE rules），加上会让所有写入硬报错。唯一键是 UNIQUE (epoch, cid, "index")
// （必须含分区键 epoch），所以认行的键就是这个三元组。
func FilterExistingERC20Transfers(existing, items []*po.FEvmERC20Transfer) []*po.FEvmERC20Transfer {
	type transferKey struct {
		epoch int64
		cid   string
		index int
	}

	if len(items) == 0 {
		return nil
	}
	if len(existing) == 0 {
		return items
	}

	seen := make(map[transferKey]struct{}, len(existing))
	for _, v := range existing {
		if v == nil {
			continue
		}
		seen[transferKey{epoch: v.Epoch, cid: v.Cid, index: v.Index}] = struct{}{}
	}
	if len(seen) == 0 {
		return items
	}

	out := make([]*po.FEvmERC20Transfer, 0, len(items))
	for _, v := range items {
		if v == nil {
			continue
		}
		if _, ok := seen[transferKey{epoch: v.Epoch, cid: v.Cid, index: v.Index}]; ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (e ERC20Dal) CreateERC20TransferBatch(ctx context.Context, items []*po.FEvmERC20Transfer) (err error) {
	db, err := e.DB(ctx)
	if err != nil {
		return
	}
	if len(items) == 0 {
		return
	}
	// 这张表是 PG 分区表且带 INSERT 规则（range_insert_action_rule），所以 ON CONFLICT 不可用：
	// PG 会直接报 SQLSTATE 0A000 让每一次写入都失败（实测重跑已写过的 100 个高度全红、框架
	// 无限重试）。幂等因此只能放应用层「先查后跳」：按本批 epoch 查出库里已有的
	// (epoch, cid, index)，过滤掉已写入的行再插入。重叠分片回放 / 同步重试天然跳过已有行，
	// 既不会重复写，也不会撞唯一键卡死该高度。批量大小仍为 100。
	epochSet := make(map[int64]struct{}, len(items))
	for _, v := range items {
		if v == nil {
			continue
		}
		epochSet[v.Epoch] = struct{}{}
	}
	if len(epochSet) == 0 {
		return
	}
	epochs := make([]int64, 0, len(epochSet))
	for k := range epochSet {
		epochs = append(epochs, k)
	}

	var existing []*po.FEvmERC20Transfer
	err = db.Select("epoch", "cid", "index").Where("epoch in ?", epochs).Find(&existing).Error
	if err != nil {
		return
	}

	items = FilterExistingERC20Transfers(existing, items)
	if len(items) == 0 {
		return
	}

	err = db.CreateInBatches(items, 100).Error
	return
}

func (e ERC20Dal) CreateERC20SwapInfoBatch(ctx context.Context, items []*po.FEvmERC20SwapInfo) (err error) {
	db, err := e.DB(ctx)
	if err != nil {
		return
	}
	// 该表当前既没有唯一键也没有 INSERT 规则，裸写即可；这里绝不能再加 ON CONFLICT ——
	// 一旦该表也按分区表声明（chain_partition_declare 会挂 INSERT 规则），
	// ON CONFLICT 会让写入整体报 SQLSTATE 0A000 而不是被忽略。
	err = db.CreateInBatches(items, 100).Error
	return
}

var _ repository.ERC20TokenRepo = (*ERC20Dal)(nil)

func NewERC20Dal(db *gorm.DB) *ERC20Dal {
	return &ERC20Dal{BaseDal: _dal.NewBaseDal(db)}
}
