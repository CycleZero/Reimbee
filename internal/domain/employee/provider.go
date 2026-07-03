package employee

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewEmployeeRepo,
	NewEmployeeBiz,
	NewEmployeeService,
)
