package budget

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewBudgetRepo,
	NewBudgetBiz,
)
