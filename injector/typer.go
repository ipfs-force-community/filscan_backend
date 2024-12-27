package injector

import (
	"github.com/google/wire"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/typer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

var TyperProviderSet = wire.NewSet(NewTyper)

func NewTyper(db *gorm.DB, adapter londobell.Adapter) *typer.Typer {
	return typer.NewTyper(dal.NewChangeActorTaskDal(db), adapter)
}
