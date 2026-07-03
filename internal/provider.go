package internal

import (
	"github.com/CycleZero/Reimbee/internal/domain"
	"github.com/CycleZero/Reimbee/internal/router"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	domain.ProviderSet,
	router.RouterProviderSet,
)
