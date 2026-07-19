package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// TestSend_Timeout_ErrCommandTimeout locks in the low-prio fix: a deadline
// timeout is reported as ErrCommandTimeout while still matching the underlying
// context cause.
func TestSend_Timeout_ErrCommandTimeout(t *testing.T) {
	ft := newFakeTransport() // no onWrite → response never arrives
	c := newClient(ft, nil)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := c.Send(ctx, "test.never", nil, nil)

	if !errors.Is(err, ErrCommandTimeout) {
		t.Errorf("want ErrCommandTimeout, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded in chain, got %v", err)
	}
	<-ft.out
}

// TestSubscribeAfterClose_Noop locks in the fix: subscribing after Close must
// not panic and must never fire.
func TestSubscribeAfterClose_Noop(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	_ = c.Close()

	var fired bool
	sub := c.Subscribe("ev", func(json.RawMessage) { fired = true })
	c.events.dispatch("ev", nil)
	sub.Unsubscribe() // must not panic

	time.Sleep(30 * time.Millisecond)
	if fired {
		t.Fatal("handler fired for subscription created after Close")
	}
}
