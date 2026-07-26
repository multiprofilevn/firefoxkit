//go:build windows

package input

import (
	"context"
	"testing"
	"time"
)

// TestNativeBackend_SharesOneGestureLock: every native backend drives the same
// desktop pointer, so two *different* instances (one per Browser) must contend
// on the SAME gesture lock. A per-instance lock would let 9 tiled native
// browsers interleave their gestures again.
func TestNativeBackend_SharesOneGestureLock(t *testing.T) {
	a := newNative(Config{Window: Window{Scale: 1}})
	b := newNative(Config{Window: Window{Scale: 1}})

	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = WithGesture(context.Background(), a, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	blocked := make(chan struct{})
	go func() {
		_ = WithGesture(context.Background(), b, func(context.Context) error { return nil })
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("second native backend entered a gesture while the first held it")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("gesture lock not released")
	}
}
