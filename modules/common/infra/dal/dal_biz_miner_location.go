package dal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"time"
)

func NewMinerLocationDal(db *gorm.DB) *MinerLocationDal {
	return &MinerLocationDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MinerLocationTaskRepo = (*MinerLocationDal)(nil)

type MinerLocationDal struct {
	*_dal.BaseDal
}

func (m MinerLocationDal) CleanMinerLocations(ctx context.Context, powerMiners []string) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`delete from chain.miner_locations where miner is not in ?`, powerMiners).Error
	if err != nil {
		return
	}
	return
}

func (m MinerLocationDal) SaveMinerLocation(ctx context.Context, item *po.MinerLocation) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`
		INSERT INTO chain.miner_locations (miner, country, city, region, latitude, longitude, updated_at, ip, multi_addrs)
		VALUES (?, ?,?, ?, ?, ?, ?, ?, ?)
		on conflict(miner) do update set ip=?,multi_addrs = ?;
   `, item.Miner, item.Country, item.City, item.Region, item.Latitude, item.Longitude, item.UpdatedAt, item.Ip, item.MultiAddrs,
		item.Ip, item.MultiAddrs).Error
	if err != nil {
		return
	}
	return
}

func (m MinerLocationDal) GetUpdateMinerLocations(ctx context.Context, before time.Time, limit int64) (locations []*po.MinerLocation, err error) {
	
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Raw(`
	select * from chain.miner_locations where ip != '' and (updated_at < ? or updated_at is null) order by updated_at limit ?`,
		before, limit).Find(&locations).Error
	if err != nil {
		return
	}
	
	return
}

func (m MinerLocationDal) UpdateMinerIp(ctx context.Context, item *po.MinerLocation) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	err = tx.Exec(`
		update chain.miner_locations set country=?,city=?,region=?,latitude=?,longitude=?,updated_at=? where miner=?`,
		item.Country, item.City, item.Region, item.Latitude, item.Longitude, item.UpdatedAt, item.Miner,
	).Error
	if err != nil {
		return
	}
	
	return
}

func (m MinerLocationDal) GetLatestMinerMultiAddrs(ctx context.Context) (addrs []*bo.MinerIpAddr, err error) {
	
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	minerEpoch := &po.SyncMinerEpochPo{}
	err = tx.Order("epoch desc").First(minerEpoch).Error
	if err != nil {
		return
	}
	var items []*po.MinerInfo
	err = tx.Select("miner,ips").Where("epoch = ? and ips is not null", minerEpoch.Epoch).Find(&items).Error
	if err != nil {
		return
	}
	
	for _, v := range items {
		addrs = append(addrs, &bo.MinerIpAddr{
			Epoch:      minerEpoch.Epoch,
			Miner:      v.Miner,
			MultiAddrs: v.Ips,
		})
	}
	
	return
}
