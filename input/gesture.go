package input

import (
	"context"
	"sync"
	"time"
)

// nativeGestureMu serializes a WHOLE gesture (move → press → release) across
// every native backend in the process. There is exactly one desktop pointer,
// so two Browsers driving it must take turns per gesture, not per syscall:
// with per-syscall locking only, browser B's SetCursorPos can land between
// browser A's move and A's button-down, dropping A's click into B's window.
// Real production shape: up to 9 native Browsers with windows tiled via
// SetWindowBounds. Cost of the coarser lock is accepted — native input is
// physically serial anyway.
var nativeGestureMu sync.Mutex

// gestureLocker is implemented by backends that drive a single OS-wide input
// resource. Only the native backend does; the BiDi backend addresses its own
// browsing context and MUST stay parallel, so it deliberately does not
// implement this.
type gestureLocker interface {
	lockGesture()
	unlockGesture()
}

// gestureHeldKey marks a context that is already inside a gesture. The gesture
// mutex is a plain sync.Mutex (non-reentrant), and composites nest — Drag holds
// the gesture across an approach glide that itself calls Cursor.MoveTo — so the
// inner call must pass through instead of self-deadlocking. All native backends
// share nativeGestureMu, so "held" is unambiguous across backend instances.
type gestureHeldKey struct{}

// WithGesture runs fn as one atomic input gesture on backends that own a shared
// OS pointer, and as a plain call on everything else. fn receives a context
// marked as in-gesture; it MUST pass that ctx down so nested composites don't
// re-acquire the lock.
func WithGesture(ctx context.Context, b Backend, fn func(context.Context) error) error {
	gl, ok := b.(gestureLocker)
	if !ok || ctx.Value(gestureHeldKey{}) != nil {
		return fn(ctx)
	}
	gl.lockGesture()
	defer gl.unlockGesture()
	return fn(context.WithValue(ctx, gestureHeldKey{}, struct{}{}))
}

// releaseGrace bounds the fallback release when the caller ctx is already dead.
const releaseGrace = 2 * time.Second

// ReleaseCtx returns the context a button release must run on. A cancelled
// caller ctx must NEVER leave the physical button pressed — the OS (native) or
// the browser (BiDi) has no idea the Go side gave up, so the button stays down
// until something else clicks. Live ctx passes through unchanged; dead ctx gets
// a short background one. Always call the returned cancel.
func ReleaseCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.Background(), releaseGrace)
}
