package calc_fns_task

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
)

func NewStore(repo repository.FnsSaver) *Store {
	return &Store{repo: repo}
}

type Store struct {
	repo           repository.FnsSaver
	hasMoveChanged bool
}

func (s *Store) SaveToken(ctx context.Context, epoch int64, items []*po.FNSToken) (err error) {

	for _, v := range items {
		var vv *po.FNSToken
		vv, err = s.repo.GetTokenOrNil(ctx, v.Name, v.Provider)
		if err != nil {
			return
		}

		var action int
		if vv != nil {
			err = s.repo.DeleteTokenByName(ctx, v.Name, v.Provider)
			if err != nil {
				return
			}
			action = po.FNSActionUpdate
		} else {
			action = po.FNSActionNew
		}

		err = s.repo.AddAction(ctx, &po.FNSAction{
			Epoch:    epoch,
			Name:     v.Name,
			Provider: v.Provider,
			Action:   action,
		})
		if err != nil {
			return
		}

		err = s.repo.AddToken(ctx, v)
		if err != nil {
			return
		}
	}

	return
}

func (s *Store) SaveTransfer(ctx context.Context, transfer *po.FNSTransfer) (err error) {
	err = s.repo.AddTransfer(ctx, transfer)
	if err != nil {
		return
	}
	return
}
