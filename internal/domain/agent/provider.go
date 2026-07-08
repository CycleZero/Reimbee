// Package agent Wire 依赖注入
package agent

import (
	agenttools "github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	DefaultConfig,
	agenttools.ProviderSet,
	MustNewModel,
	NewReimburseAgent,
	NewAgentService,
)
