package domain

import (
	"github.com/CycleZero/Reimbee/internal/domain/demo"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	demo.ProviderSet,
	NewServiceHub,
)
