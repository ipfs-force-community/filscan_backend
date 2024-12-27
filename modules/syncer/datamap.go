package syncer

import (
	"fmt"
	"sync"
)

type DataKey = string

type Datamap struct {
	lock sync.Mutex
	data map[DataKey]any
	init map[DataKey]struct{}
}

// Get 获取数据
// 如果 Keys 对应的值没有被初始化过，则会报错，以此来处理依赖其它任务数据项的情景
func (d *Datamap) Get(key DataKey) (val any, err error) {
	d.lock.Lock()
	defer func() {
		d.lock.Unlock()
	}()
	
	if _, ok := d.init[key]; !ok {
		err = fmt.Errorf("can't find key: %s in datamap", key)
		return
	}
	
	val = d.data[key]
	
	return
}

// Set 存放数据值
// 如果相同的 Keys 被重复存放，则报错
func (d *Datamap) Set(key DataKey, val any) (err error) {
	d.lock.Lock()
	defer func() {
		d.lock.Unlock()
	}()
	
	if d.data == nil {
		d.data = map[DataKey]any{}
	}
	if d.init == nil {
		d.init = map[DataKey]struct{}{}
	}
	
	if _, ok := d.init[key]; ok {
		err = fmt.Errorf("duplicated key: %s in datamap", key)
		return
	}
	
	d.data[key] = val
	d.init[key] = struct{}{}
	
	return
}
