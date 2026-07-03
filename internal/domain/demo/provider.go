package demo

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewDemoRepo,
	NewDemoBiz,
	NewDemoService,
)
