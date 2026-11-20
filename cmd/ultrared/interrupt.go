package main

import (
	"context"
	"sync"
)

// interrupter turns Ctrl-C into cancellation of the current turn only.
// Each turn gets its own context, so an interrupted turn does not poison the
// ones after it (a single signal.NotifyContext for the whole session did).
type interrupter struct {
	mu     sync.Mutex
	cancel context.CancelFunc // nil while waiting at the prompt
}

// turn returns a context for one user turn. Call done when the turn ends.
func (i *interrupter) turn(parent context.Context) (ctx context.Context, done context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	i.mu.Lock()
	i.cancel = cancel
	i.mu.Unlock()
	return ctx, func() {
		i.mu.Lock()
		i.cancel = nil
		i.mu.Unlock()
		cancel()
	}
}

// interrupt cancels the running turn. It reports false if no turn is running.
func (i *interrupter) interrupt() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cancel == nil {
		return false
	}
	i.cancel()
	i.cancel = nil
	return true
}
