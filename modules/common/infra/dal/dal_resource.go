package dal

import (
	"context"

	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

var _ repository.ResourceRepo = (*ResourceDal)(nil)

type ResourceDal struct {
	*_dal.BaseDal
}

func (r ResourceDal) GetFEvmItemsByCategory(ctx context.Context, category string) ([]*po.FEvmItem, []*po.FEvmItemCategory, error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return nil, nil, err
	}

	res2 := []*po.FEvmItemCategory{}
	if category == "all" {
		err = tx.Find(&res2).Error
	} else {
		err = tx.Find(&res2, "category = ?", category).Error
	}

	if err != nil {
		return nil, nil, err
	}

	ids := []int{}
	for i := range res2 {
		ids = append(ids, res2[i].ItemId)
	}

	res1 := []*po.FEvmItem{}
	err = tx.Find(&res1, "id in (?)", ids).Error
	return res1, res2, err
}

func (r ResourceDal) GetFEvmCategorys(ctx context.Context) ([]string, []int, error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return nil, nil, err
	}
	var res1 = []struct {
		Count    int
		Category string
	}{}
	err = tx.Raw("SELECT count(1), category FROM fevm_item_category GROUP BY category").Scan(&res1).Error
	if err != nil {
		return nil, nil, err
	}
	resString := []string{}
	resCount := []int{}
	for i := range res1 {
		resString = append(resString, res1[i].Category)
		resCount = append(resCount, res1[i].Count)
	}
	return resString, resCount, nil
}

func (r ResourceDal) GetHotItems(ctx context.Context) ([]*po.FEvmItem, []*po.FEvmHotItem, error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return nil, nil, err
	}

	res2 := []*po.FEvmHotItem{}
	err = tx.Find(&res2).Error

	if err != nil {
		return nil, nil, err
	}

	ids := []int{}
	for i := range res2 {
		ids = append(ids, res2[i].ItemId)
	}

	res1 := []*po.FEvmItem{}
	err = tx.Find(&res1, "id in (?)", ids).Error
	return res1, res2, err
}

func NewResourceDal(db *gorm.DB) *ResourceDal {
	return &ResourceDal{BaseDal: _dal.NewBaseDal(db)}
}

func (r ResourceDal) GetBannerByCategoryAndLanguage(ctx context.Context, category, language string) ([]*po.Banner, error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return nil, err
	}
	res := []*po.Banner{}
	err = tx.Find(&res, "category = ? and language = ?", category, language).Error
	return res, err
}
