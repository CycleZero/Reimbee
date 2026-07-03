package reimbursement

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewReimbursementRepo,
	NewReimbursementBiz,
	NewReimbursementService,
)
