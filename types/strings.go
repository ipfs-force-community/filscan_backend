package types

import (
	"context"
	"database/sql/driver"
	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
	"reflect"
	"strings"
)

// StringArray 将string数组映射成数据库中的数组
type StringArray []string

// GormDataType gorm common data type
// 这个必须要定义，不定义框架无法自动序列化
func (StringArray) GormDataType() string {
	return "array"
}

// GormDBDataType gorm db data type
// 这个貌似没卵用，下面的定义也不是JSONB，数据库中定义的是Varchar[]数组
func (StringArray) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "JSON"
	case "mysql":
		return "JSON"
	case "postgres":
		return "JSONB"
	}
	return ""
}

// GormValue 必须要定义，自动组装结构体时会用到。不定义可能会报错
func (b StringArray) GormValue(_ context.Context, db *gorm.DB) clause.Expr {
	if len(b) == 0 {
		return gorm.Expr("NULL")
	}
	
	data := ArrayToJSONB(b)
	
	switch db.Dialector.Name() {
	case "mysql":
		if v, ok := db.Dialector.(*mysql.Dialector); ok && !strings.Contains(v.ServerVersion, "MariaDB") {
			return gorm.Expr("CAST(? AS JSON)", string(data))
		}
	}
	
	return gorm.Expr("?", string(data))
}

func (b StringArray) Value() (driver.Value, error) {
	if len(b) == 0 {
		return nil, nil
	}
	
	data := ArrayToJSONB(b)
	return data, nil
}

// Scan Gorm框架读取的时候会用到
func (b *StringArray) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	
	var record string
	switch value.(type) {
	case string:
		record = value.(string)
	case []byte:
		record = string(value.([]byte))
	default:
		return errors.Errorf("unknown type(type: %v), value: %v", reflect.TypeOf(value), value)
	}
	
	array := JSONBToArray(record)
	*b = array
	return nil
}

func ArrayToJSONB(array []string) []byte {
	builder := strings.Builder{}
	builder.WriteString("{\"")
	builder.WriteString(strings.Join(array, "\",\""))
	builder.WriteString("\"}")
	return []byte(builder.String())
}

func JSONBToArray(jsonb string) []string {
	jsonb = strings.TrimPrefix(jsonb, "{")
	jsonb = strings.TrimSuffix(jsonb, "}")
	values := strings.Split(jsonb, ",")
	var array []string
	for _, item := range values {
		array = append(array, strings.Trim(item, "\""))
	}
	return array
}
