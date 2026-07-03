//go:build wireinject
// +build wireinject

package main

import (
	"gin-template/infra"
	"gin-template/internal"
	"gin-template/log"

	"github.com/google/wire"
	"github.com/spf13/viper"
)

func initApp(vc *viper.Viper, logger *log.Logger) *MainApp {
	panic(wire.Build(
		NewMainApp,
		infra.ProviderSet,
		internal.ProviderSet,
	))
}
