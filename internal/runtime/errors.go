package runtime

import "errors"

var (
	ErrProposalRequired    = errors.New("runtime: proposal required")
	ErrRunCancelled        = errors.New("runtime: run cancelled")
	ErrDuplicateInboxItem  = errors.New("runtime: duplicate inbox item (idempotency key already accepted)")
)
