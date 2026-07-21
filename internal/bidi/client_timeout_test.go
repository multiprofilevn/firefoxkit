package bidi

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

// TestSend_NoDeadline_BackstopTimeout: with a configured backstop and a caller
// ctx WITHOUT deadline, a lost response must not park Send forever — it must
// return ErrCommandTimeout within the backstop, clean pending, and release the
// goroutine.
func TestSend_NoDeadline_BackstopTimeout(t *testing.T) {
	ft := newFakeTransport()
	// No onWrite — response never arrives on an otherwise-alive transport.
	c := newClient(ft, nil)
	c.defaultCallTimeout = 50 * time.Millisecond
	defer c.Close()

	baseline := runtime.NumGoroutine()

	start := time.Now()
	err := c.Send(context.Background(), "test.never", nil, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("expected ErrCommandTimeout, got %v", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("Send took %v, expected < 200ms", elapsed)
	}

	c.mu.Lock()
	got := len(c.pending)
	c.mu.Unlock()
	if got != 0 {
		t.Fatalf("pending map leaked %d entries", got)
	}

	// Goroutine count back to baseline after settle (context.WithTimeout timer
	// goroutines etc. wind down).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > baseline {
		t.Fatalf("goroutines did not settle: baseline=%d now=%d", baseline, n)
	}
	<-ft.out // drain the written command
}

// TestSend_CallerDeadline_NotOverridden: caller's own (shorter) deadline wins;
// the backstop must not replace it.
func TestSend_CallerDeadline_NotOverridden(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	c.defaultCallTimeout = 10 * time.Second
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.Send(ctx, "test.never", nil, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("expected ErrCommandTimeout, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Send took %v — caller 20ms deadline was overridden by backstop", elapsed)
	}
	<-ft.out
}

// TestSend_BackstopDisabled_LegacyPark: defaultCallTimeout=0 keeps legacy
// behavior — Send parks until Close() drains it with ErrClientClosed.
func TestSend_BackstopDisabled_LegacyPark(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	// defaultCallTimeout stays zero — backstop disabled.

	done := make(chan error, 1)
	go func() {
		done <- c.Send(context.Background(), "test.never", nil, nil)
	}()

	// Send must NOT return on its own.
	select {
	case err := <-done:
		t.Fatalf("Send returned before Close with backstop disabled: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := c.Close(); err != nil {
		t.Logf("close err (ignored): %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrClientClosed) {
			t.Fatalf("expected ErrClientClosed after Close, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Send did not return after Close")
	}
}
