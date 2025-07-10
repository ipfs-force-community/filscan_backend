package syncer

import (
	"fmt"

	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type Name = string

func NewManager(enable map[string]struct{}, syncers []*Syncer) *Manager {
	return &Manager{
		syncers: syncers,
		log:     logging.NewLogger("manager"),
		enable:  enable,
	}
}

type Manager struct {
	syncers []*Syncer
	check   map[Name]struct{}
	log     *logging.Logger
	enable  map[string]struct{}
}

func (s *Manager) SyncerHeights() (heights map[Name]chain.Epoch) {
	heights = map[Name]chain.Epoch{}
	for _, v := range s.syncers {
		heights[v.name] = v.epoch
	}
	return
}

func (s *Manager) Run() (err error) {

	s.log.Infof("启动同步管理器")
	s.check = map[Name]struct{}{}

	syncers := map[string]struct{}{}
	for _, v := range s.syncers {
		syncers[v.name] = struct{}{}
	}

	for k := range s.enable {
		if _, ok := syncers[k]; !ok {
			err = fmt.Errorf("enable syncer: %s is not register", k)
			return
		}
	}

	for _, v := range s.syncers {
		if _, ok := s.check[v.name]; ok {
			err = fmt.Errorf("syncer name: %s confilct", v.name)
			return
		}

		if _, ok := s.enable[v.name]; !ok {
			s.log.Warnf("忽略同步器: %s", v.name)
			continue
		}

		s.log.Infof("启动同步器: %s", v.name)
		err = v.Init()
		if err != nil {
			return
		}
		s.check[v.name] = struct{}{}

		vv := v
		go vv.Run()
	}

	return
}
