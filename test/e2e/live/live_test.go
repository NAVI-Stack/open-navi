//go:build e2e && e2e_live

package live

import (
	"os"
	"testing"
)

func TestLiveAcceptanceLaneRequiresEnv(t *testing.T) {
	if os.Getenv("NAVI_E2E_LIVE") == "1" {
		t.Skip("live acceptance scaffolding is in place; add provider credentials and live scenarios to enable this lane")
	}
	t.Skip("set NAVI_E2E_LIVE=1 and provide live credentials to enable real-provider acceptance tests")
}
