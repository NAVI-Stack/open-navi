package hooks

import (
	"context"
	"sort"
	"sync"
)

// Runner runs typed hooks in priority order.
type Runner struct {
	mu    sync.RWMutex
	hooks map[HookName][]Registration
}

// NewRunner creates a new hook runner.
func NewRunner() *Runner {
	return &Runner{
		hooks: make(map[HookName][]Registration),
	}
}

// Register adds a handler for the given hook name. Lower priority runs first.
func (r *Runner) Register(name HookName, priority int, handler HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[name] = append(r.hooks[name], Registration{Priority: priority, Handler: handler})
	sort.Slice(r.hooks[name], func(i, j int) bool {
		return r.hooks[name][i].Priority < r.hooks[name][j].Priority
	})
}

// Run invokes all handlers for the hook in priority order. Payload is passed through the chain.
// If a handler returns an error, Run stops and returns that error.
func (r *Runner) Run(ctx context.Context, name HookName, payload interface{}) (interface{}, error) {
	r.mu.RLock()
	regs := make([]Registration, len(r.hooks[name]))
	copy(regs, r.hooks[name])
	r.mu.RUnlock()

	current := payload
	for _, reg := range regs {
		next, err := reg.Handler(ctx, current)
		if err != nil {
			return nil, err
		}
		if next != nil {
			current = next
		}
	}
	return current, nil
}

// RunNoPayload runs hooks that don't need to pass or modify payload (e.g. side-effect only).
// Errors from handlers are returned; the first error stops the chain.
func (r *Runner) RunNoPayload(ctx context.Context, name HookName) error {
	_, err := r.Run(ctx, name, nil)
	return err
}
