package internal

import (
	"gin-template/internal/domain"
	"gin-template/internal/router"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	domain.ProviderSet,
	router.RouterProviderSet,
)
