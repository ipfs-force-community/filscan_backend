package _app

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewGormDB(dsn string, opts ...gorm.Option) (*gorm.DB, error) {
	_opts := make([]gorm.Option, 0)
	if len(opts) == 0 {
		_opts = append(_opts, &gorm.Config{
			Logger: logger.Default,
		})
	} else {
		_opts = append(_opts, opts...)
	}
	db, err := gorm.Open(postgres.Open(dsn), _opts...)
	if err != nil {
		err = fmt.Errorf("connect postgres error: %s", err)
		return nil, err
	}
	_db, err := db.DB()
	if err != nil {
		err = fmt.Errorf("get postgres sql.DB error: %s", err)
		return nil, err
	}
	err = _db.Ping()
	if err != nil {
		err = fmt.Errorf("ping postgres error:%s", err)
		return nil, err
	}
	return db, nil
}
