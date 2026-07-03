package department

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewDepartmentRepo,
	NewDepartmentBiz,
	NewDepartmentService,
)
