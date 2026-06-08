//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ceoai/navi/test/e2e/harness"
)

var suiteStack *harness.Stack

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

func runSuite(m *testing.M) int {
	repoRoot, err := harness.FindRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find repo root: %v\n", err)
		return 1
	}

	stack, err := harness.NewStack(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create e2e stack: %v\n", err)
		return 1
	}
	defer func() { _ = stack.Cleanup() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if err := stack.Up(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "start e2e stack: %v\n", err)
		return 1
	}
	defer func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer downCancel()
		_ = stack.Down(downCtx)
	}()

	if err := stack.Claim(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "claim e2e instance: %v\n", err)
		return 1
	}

	suiteStack = stack
	return m.Run()
}
