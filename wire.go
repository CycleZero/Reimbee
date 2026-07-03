//go:build wireinject
// +build wireinject

package main

import (
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal"
	"github.com/CycleZero/Reimbee/log"

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
