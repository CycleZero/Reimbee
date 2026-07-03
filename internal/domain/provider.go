package domain

import (
	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewServiceHub,

	approval.ProviderSet,
	budget.ProviderSet,
	employee.ProviderSet,
	reimbursement.ProviderSet,
)
