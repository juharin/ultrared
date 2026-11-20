package main

import (
	"context"
	"testing"
)

// Regression test: one Ctrl-C used to cancel a session-wide context, so every
// later turn failed with "context canceled".
func TestInterruptCancelsOnlyCurrentTurn(t *testing.T) {
	var intr interrupter

	ctx1, done1 := intr.turn(context.Background())
	if !intr.interrupt() {
		t.Fatal("interrupt during a turn reported no active turn")
	}
	if ctx1.Err() == nil {
		t.Fatal("interrupt did not cancel the running turn")
	}
	done1()

	ctx2, done2 := intr.turn(context.Background())
	defer done2()
	if err := ctx2.Err(); err != nil {
		t.Fatalf("next turn started already cancelled: %v", err)
	}
}

func TestInterruptAtPromptCancelsNothing(t *testing.T) {
	var intr interrupter
	if intr.interrupt() {
		t.Fatal("interrupt with no turn running reported an active turn")
	}

	ctx, done := intr.turn(context.Background())
	done() // turn finished normally
	if intr.interrupt() {
		t.Fatal("interrupt after the turn ended reported an active turn")
	}
	_ = ctx
}
