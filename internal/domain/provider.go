package domain

import (
	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/auth"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/compliance"
	"github.com/CycleZero/Reimbee/internal/domain/department"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewServiceHub,

	agent.ProviderSet,
	approval.ProviderSet,
	auth.ProviderSet,
	budget.ProviderSet,
	compliance.ProviderSet,
	department.ProviderSet,
	employee.ProviderSet,
	reimbursement.ProviderSet,
)
