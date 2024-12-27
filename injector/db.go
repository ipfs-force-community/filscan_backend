package injector

import (
	"fmt"
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gorm.io/driver/postgres"
	
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"time"
)

var GormProviderSet = wire.NewSet(NewGormDB)

func NewGormDB(conf *config.Config) (*gorm.DB, func(), error) {
	
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             5 * time.Second, // Slow SQL threshold
			LogLevel:                  logger.Error,    // Log level
			IgnoreRecordNotFoundError: true,            // Ignore ErrRecordNotFound error for logger
			Colorful:                  true,            // Disable color
		},
	)
	
	db, err := newGormDB(*conf.DSN, &gorm.Config{
		Logger:                   newLogger,
		DisableNestedTransaction: true,
	})
	if err != nil {
		return nil, nil, err
	}
	_db, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	
	_db.SetMaxOpenConns(500)
	_db.SetConnMaxIdleTime(30 * time.Second)
	
	return db, func() {
		d, _ := db.DB()
		if d != nil {
			_ = d.Close()
		}
	}, nil
}

func newGormDB(dsn string, opts ...gorm.Option) (*gorm.DB, error) {
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
