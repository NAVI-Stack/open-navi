package gateway

import (
	"context"
	"errors"
	"net/http"
	"time"

	naviruntime "github.com/ceoai/navi/internal/runtime"
)

// naviMessageIngressTimeout bounds the full gateway ingress path for session
// message submission, including pre-queue visibility/governance checks.
var naviMessageIngressTimeout = naviruntime.DefaultSubmitMessageTimeout

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining <= timeout {
			return context.WithCancel(ctx)
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func newNaviMessageIngressContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return withOptionalTimeout(ctx, naviMessageIngressTimeout)
}

func replyIngressTimeout(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	replyError(w, http.StatusGatewayTimeout, context.DeadlineExceeded.Error())
	return true
}
