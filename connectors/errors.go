package connectors

import "errors"

// Classified errors for connector Send operations. The Manager uses these
// to determine retry strategy:
//   - ErrNotRunning / ErrSendFailed: permanent — no retry
//   - ErrRateLimit: retry after a fixed delay
//   - ErrTemporary: retry with exponential backoff
var (
	ErrNotRunning = errors.New("connector: not running")
	ErrSendFailed = errors.New("connector: send failed")
	ErrRateLimit  = errors.New("connector: rate limited")
	ErrTemporary  = errors.New("connector: temporary error")
)
