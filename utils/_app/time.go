package _app

import (
	"github.com/golang-module/carbon/v2"
	"gorm.io/gorm"
	"time"
)

var TimeLocation *time.Location

const TimeLayout = carbon.RFC3339Layout

func init() {
	TimeLocation, _ = time.LoadLocation(carbon.UTC)
}

func CheckTimezone(db *gorm.DB) (err error) {
	return
}

// 检查运行环境的时区
func checkRuntimeTimezone() {

}

// 检查数据库的时区
func checkDatabaseTimezone(db *gorm.DB) (err error) {
	return
}

// 时间校准
func CheckTime() {

}

// 校准数据库时间
func checkDatabaseTime() (err error) {
	return
}
