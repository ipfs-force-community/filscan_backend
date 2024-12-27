package mpo

import (
	"time"

	"gorm.io/gorm"
)

type Rule struct {
	ID           uint
	UserID       int64
	GroupID      int64  // 数据库中存取这些，在实现时候，如果miner为all时候，可以联动去查询group。然后保存
	MinerIDOrAll string `gorm:"column:miner_id_or_all"` // todo 注意生成一个 类似-1 的一个组id，方便默认情况。group_id为-1时候表示所有的节点。 在内存中运行，用个map或什么来保存所有要检查规则的miner
	AccountType  string //在余额部分用于区分是owner、worker还是什么东西
	AccountAddr  string //设计：有该字段时候，查找该字段对应的数据。无该字段时候，找上面标签的内容。目的：方便在选择all的时候，也能使得逻辑判断下去。（如all下面的controller，不指定具体miner下哪个c，则所有。）
	MonitorType  string
	Uuid         string
	Operator     *string
	Operand      *string
	MailAlert    *string
	MsgAlert     *string
	CallAlert    *string
	Interval     *int64 //todo 保留给未来自定义间隔接口
	IsActive     bool
	IsVip        bool
	Description  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Rule) TableName() string {
	return "monitor.rules"
}

func (r *Rule) BeforeCreate(tx *gorm.DB) (err error) {
	r.CreatedAt = time.Now()
	r.UpdatedAt = time.Now()
	return
}

func (r *Rule) BeforeUpdate(tx *gorm.DB) (err error) {
	r.UpdatedAt = time.Now()
	return
}
