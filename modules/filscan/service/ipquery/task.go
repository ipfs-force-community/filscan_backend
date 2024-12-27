package ipquery

import (
	"context"
	"strings"
	"time"

	"github.com/gozelle/async/forever"
	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
)

func NewMinerLocationTask(repo repository.MinerLocationTaskRepo) *MinerLocationTask {
	return &MinerLocationTask{
		log:   logging.NewLogger("miner-location"),
		repo:  repo,
		query: NewIpQuery(),
	}
}

type MinerLocationTask struct {
	lastExecTime time.Time
	log          *logging.Logger
	repo         repository.MinerLocationTaskRepo
	query        *IpQuery
}

func (a *MinerLocationTask) Run() {
	forever.Run(1*time.Minute, func() {
		err := a.run()
		if err != nil {
			a.log.Errorf("miner location task exec error: %s", err)
		}
	})
}

func (a *MinerLocationTask) run() (err error) {

	if time.Now().Before(a.lastExecTime.Add(24 * time.Hour)) {
		return
	}

	// 获取最新的 miner ip 列表
	addrs, err := a.repo.GetLatestMinerMultiAddrs(context.Background())
	if err != nil {
		return
	}

	var miners []string
	for _, v := range addrs {
		miners = append(miners, v.Miner)
	}

	// 清除无效的地址
	err = a.repo.CleanMinerLocations(context.Background(), miners)
	if err != nil {
		return
	}

	for _, v := range addrs {
		item := &po.MinerLocation{
			Miner:      v.Miner,
			Ip:         "",
			MultiAddrs: v.MultiAddrs,
		}
		for _, vv := range v.MultiAddrs {
			if strings.HasPrefix(vv, "/ip4") {
				strs := strings.Split(vv, "/")
				if len(strs) > 2 && strings.Contains(strs[2], ".") {
					item.Ip = strs[2]
				}
			}
		}
		if item.Ip != "" {
			err = a.repo.SaveMinerLocation(context.Background(), item)
			if err != nil {
				return
			}
		}
	}

	// 更新 10 天以前的记录
	before := time.Now().AddDate(0, 0, -10)
	queryRows, err := a.repo.GetUpdateMinerLocations(context.Background(), before, 1000)
	if err != nil {
		return
	}

	for _, row := range queryRows {
		if e := a.queryIp(row); e == nil {
			err = a.repo.UpdateMinerIp(context.Background(), row)
			if err != nil {
				return
			}
		} else {
			a.log.Errorf("query miner: %s ip: %s error: %s", row.Miner, row.Ip, e)
		}
	}

	a.log.Infof("handle %d miner locations", len(queryRows))
	a.lastExecTime = time.Now()
	return
}

func (a *MinerLocationTask) queryIp(row *po.MinerLocation) (err error) {

	res, err := a.query.Query(row.Ip)
	if err != nil {
		a.log.Errorf("查询 ip: %s 错误: %s", row.Ip, err)
		return
	}

	now := time.Now()
	row.Country = res.Country
	row.City = res.City
	row.Region = res.Region
	row.Longitude = res.Longitude
	row.Latitude = res.Latitude
	row.UpdatedAt = &now

	return
}
