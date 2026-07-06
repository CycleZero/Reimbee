//go:build wireinject
// +build wireinject

package main

import (
	"github.com/CycleZero/Reimbee/internal/domain/agent"
	agentgraph "github.com/CycleZero/Reimbee/internal/domain/agent/graph"
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal"
	"github.com/CycleZero/Reimbee/log"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/wire"
	"github.com/spf13/viper"
)

func initApp(vc *viper.Viper, logger *log.Logger) *MainApp {
	panic(wire.Build(
		NewMainApp,
		infra.ProviderSet,
		internal.ProviderSet,
		agentgraph.ProviderSet,
		// 将 Graph 的 RootGraphRunnable 绑定到 AgentRunner 需要的 Runnable 接口
		wire.Bind(
			new(compose.Runnable[agent.AgentInput, *schema.Message]),
			new(*agentgraph.RootGraphRunnable),
		),
	))
}
