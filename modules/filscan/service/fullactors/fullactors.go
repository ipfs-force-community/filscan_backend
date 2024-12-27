package fullactors

import (
	"context"
	"time"

	"github.com/gozelle/async/forever"
	"github.com/gozelle/async/parallel"
	logging "github.com/gozelle/logger"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewSyncer(repo repository.ActorSyncSaver, agg londobell.Agg) *Syncer {
	return &Syncer{
		repo: repo,
		agg:  agg,
		log:  logging.NewLogger("fullactors"),
	}
}

type Syncer struct {
	repo repository.ActorSyncSaver
	agg  londobell.Agg
	log  *logging.Logger
}

func (s *Syncer) Sync() {
	forever.Run(30*time.Minute, func() {
		err := s.sync()
		if err != nil {
			s.log.Errorf("同步 actor 创建时间错误: %s", err)
			return
		}
	})
}

func (s *Syncer) sync() (err error) {

	actors, err := s.repo.GetNoneCreatedTimeActors(context.Background())
	if err != nil {
		return
	}

	if len(actors) == 0 {
		s.log.Infof("没有 actor 需要同步 CreateTime")
		return
	}

	gen := func(actor *po.ActorPo) parallel.Runner[parallel.Null] {
		return func(ctx context.Context) (parallel.Null, error) {
			var epoch chain.Epoch
			epoch, err = s.agg.CreateTime(ctx, chain.SmartAddress(actor.Id))
			if err != nil {
				return nil, err
			}

			if epoch <= 0 {
				//s.log.Warnf("cant't fetch actor: %s create time", actor.Id)
				return nil, nil
			}

			t := epoch.Time()
			actor.CreatedTime = &t
			return nil, nil
		}
	}

	var runners []parallel.Runner[parallel.Null]
	for _, v := range actors {
		runners = append(runners, gen(v))
	}

	results := parallel.Run[parallel.Null](context.Background(), 20, runners)
	err = parallel.Wait[parallel.Null](results, nil)
	if err != nil {
		return
	}

	count := 0
	for _, v := range actors {
		if v.CreatedTime != nil {
			err = s.repo.UpdateActorCreateTime(context.Background(), v)
			if err != nil {
				return
			}
			count++
		}
	}

	if count > 0 {
		s.log.Infof("更新了 %d Actor", count)
	}

	return
}
