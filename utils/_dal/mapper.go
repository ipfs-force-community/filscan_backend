package _dal

import (
	"context"
	"fmt"
	"gorm.io/gorm"
)

func NewBaseDal(db *gorm.DB) *BaseDal {
	return &BaseDal{db: db}
}

type BaseDal struct {
	db *gorm.DB
}

func ContextWithDB(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, CONTEXT_DB_KEY, db)
}

func IsTx(ctx context.Context) bool {
	v := ctx.Value(CONTEXT_DB_KEY)
	if v == nil {
		return false
	}
	_, ok := v.(*gorm.DB)
	return ok
}

// DB 自动获取 DB 对象
func (m *BaseDal) DB(ctx context.Context) (*gorm.DB, error) {
	if v := ctx.Value(CONTEXT_DB_KEY); v != nil {
		vv, ok := v.(*gorm.DB)
		if !ok {
			return nil, fmt.Errorf("context CONTEXT_DB_KEY expect *gorm.DB")
		}
		return vv, nil
	}
	return m.db, nil
}

func (m *BaseDal) Exec(ctx context.Context, handle func(tx *gorm.DB) error) (err error) {
	if handle == nil {
		return
	}
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = handle(tx)
	return
}
