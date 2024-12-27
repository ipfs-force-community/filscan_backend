package calc_change_actor_task_test

import (
	"github.com/gozelle/spew"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_change_actor_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-change-actor-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"testing"
	
	"github.com/gozelle/fs"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestNewComer(t *testing.T) {
	
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)
	
	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	
	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)
	
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	
	task := calc_change_actor_task.NewCalcChangeActorTask(dal.NewChangeActorTaskDal(db))
	
	preEpoch := chain.Epoch(2965576)
	actor, err := task.PrepareActor(syncer.NewTestContext(adapter, agg, 2965577), "f02227281", preEpoch)
	require.NoError(t, err)
	
	spew.Json(actor)
}
