//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"github.com/google/wire"
	"github.com/gozelle/gin"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
)

func wireApp(conf *config.Config) (*gin.Engine, func(), error) {
	panic(wire.Build(
		injector.APIProvider,
		injector.LondoBellAdapterProviderSet,
		injector.LondoBellAggProviderSet,
		injector.GormProviderSet,
		injector.NewAbiDecoderSet,
		injector.TyperProviderSet,
		injector.MailClientSet,
		injector.ProSet,
		injector.RedisSet,
		newApp,
	))
}
