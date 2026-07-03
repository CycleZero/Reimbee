package domain

import (
	"gin-template/internal/domain/demo"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	demo.ProviderSet,
	NewServiceHub,
)
