package tui

import (
	"context"
	"errors"
	"testing"
	"time"
)

// esc must abort the in-flight command and say so, and the cancelled
// reply that follows must not paint "context canceled" as a query error.
func TestEscCancelsInFlightCommand(t *testing.T) {
	m := browseModel(t)
	m.loading = true
	ctx := m.newOpContext(time.Minute)
	if ctx.Err() != nil {
		t.Fatalf("a fresh op context must be live: %v", ctx.Err())
	}
	u, cmd := m.Update(testKey("esc"))
	m = u.(Model)
	if m.status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", m.status)
	}
	if m.loading {
		t.Fatal("esc must clear loading")
	}
	if cmd != nil {
		t.Fatal("esc must not dispatch a command")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("esc must cancel the in-flight context, got %v", ctx.Err())
	}

	// The aborted command's reply arrives afterwards.
	u, _ = m.Update(detailLoadedMsg{table: "orders", err: context.Canceled})
	got := u.(Model)
	if got.err != "" {
		t.Fatalf("cancellation must not surface as an error, got %q", got.err)
	}
	if got.loading {
		t.Fatal("the cancelled reply must still clear loading")
	}
}
