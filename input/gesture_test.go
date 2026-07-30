package input

import (
	"context"
	"sync"
	"testing"
	"time"
)

// gestureEvent is one recorded backend op tagged with the gesture that emitted
// it ("A#0" = browser A, gesture 0).
type gestureEvent struct {
	tag string
	op  string
}

// gestureRig stands in for the process-wide native input layer: syscallMu is
// the per-syscall lock the backend already had (nativeInputMu), gestureMu is
// the gesture-scope lock every native backend shares (nativeGestureMu).
type gestureRig struct {
	syscallMu sync.Mutex
	gestureMu sync.Mutex

	mu     sync.Mutex
	events []gestureEvent
}

func (r *gestureRig) record(tag, op string) {
	r.syscallMu.Lock()
	defer r.syscallMu.Unlock()
	r.mu.Lock()
	r.events = append(r.events, gestureEvent{tag: tag, op: op})
	r.mu.Unlock()
}

func (r *gestureRig) snapshot() []gestureEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]gestureEvent, len(r.events))
	copy(out, r.events)
	return out
}

// gestureFake is one "browser": its own Backend, all writing into the shared
// rig. tag is set by the owning goroutine before each gesture, so every op the
// cursor engine emits carries the gesture it belongs to.
type gestureFake struct {
	rig *gestureRig
	tag string
}

func (f *gestureFake) MouseMove(_ context.Context, _, _ float64) error {
	f.rig.record(f.tag, "move")
	return nil
}

func (f *gestureFake) MouseDown(_ context.Context, _, _ float64, _ MouseButton) error {
	f.rig.record(f.tag, "down")
	return nil
}

func (f *gestureFake) MouseUp(_ context.Context, _, _ float64, _ MouseButton) error {
	f.rig.record(f.tag, "up")
	return nil
}

func (f *gestureFake) MouseScroll(_ context.Context, _, _, _, _ float64) error {
	f.rig.record(f.tag, "scroll")
	return nil
}
func (f *gestureFake) KeyChar(context.Context, rune) error              { return nil }
func (f *gestureFake) InsertText(context.Context, string) error         { return nil }
func (f *gestureFake) KeyCombo(context.Context, uint16, Modifier) error { return nil }

func (f *gestureFake) lockGesture()   { f.rig.gestureMu.Lock() }
func (f *gestureFake) unlockGesture() { f.rig.gestureMu.Unlock() }

var (
	_ Backend       = (*gestureFake)(nil)
	_ gestureLocker = (*gestureFake)(nil)
)

// assertNoInterleave fails when the ops of any gesture are not one contiguous
// run in the recorded stream — i.e. another browser's event landed inside it.
func assertNoInterleave(t *testing.T, events []gestureEvent) {
	t.Helper()
	first := map[string]int{}
	last := map[string]int{}
	count := map[string]int{}
	for i, e := range events {
		if _, seen := first[e.tag]; !seen {
			first[e.tag] = i
		}
		last[e.tag] = i
		count[e.tag]++
	}
	for tag, n := range count {
		if span := last[tag] - first[tag] + 1; span != n {
			t.Fatalf("gesture %s interleaved: %d ops spread over %d slots\nstream=%v", tag, n, span, events)
		}
	}
}

// TestGestureLock_ClicksDoNotInterleave: two Cursors over two native-like
// backends sharing one OS pointer must not weave their move/down/up together.
// Fails with per-syscall locking only (browser B's move lands inside A's
// press-hold window and its click then targets A's coordinates).
func TestGestureLock_ClicksDoNotInterleave(t *testing.T) {
	rig := &gestureRig{}
	const gestures = 3

	var wg sync.WaitGroup
	for _, id := range []string{"A", "B"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			fake := &gestureFake{rig: rig}
			cur := NewCursorWithProfile(fake, ProfileMachine)
			for i := 0; i < gestures; i++ {
				fake.tag = id + "#" + string(rune('0'+i))
				if err := cur.Click(context.Background(), 10, 20, MouseLeft); err != nil {
					t.Errorf("%s click %d: %v", id, i, err)
					return
				}
			}
		}(id)
	}
	wg.Wait()

	events := rig.snapshot()
	if len(events) != 2*gestures*3 {
		t.Fatalf("want %d ops, got %d: %v", 2*gestures*3, len(events), events)
	}
	assertNoInterleave(t, events)
}

// TestGestureLock_DragStyleNesting: a composite that already holds the gesture
// (Drag) and then calls Cursor.MoveTo must not deadlock — the in-gesture ctx
// makes the nested acquisition a pass-through.
func TestGestureLock_DragStyleNesting(t *testing.T) {
	rig := &gestureRig{}
	fake := &gestureFake{rig: rig, tag: "A#0"}
	cur := NewCursorWithProfile(fake, ProfileMachine)

	done := make(chan error, 1)
	go func() {
		done <- WithGesture(context.Background(), fake, func(ctx context.Context) error {
			if err := cur.MoveTo(ctx, 1, 2); err != nil {
				return err
			}
			return cur.Click(ctx, 3, 4, MouseLeft)
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("nested gesture: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested gesture deadlocked on the gesture lock")
	}
}

// TestWithGesture_BiDiBackendStaysParallel: a backend without the gesture lock
// (BiDi) must never serialize — two overlapping calls both run at once.
func TestWithGesture_BiDiBackendStaysParallel(t *testing.T) {
	b := &fakeBackend{}
	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = WithGesture(context.Background(), b, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	done := make(chan struct{})
	go func() {
		_ = WithGesture(context.Background(), b, func(context.Context) error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("BiDi backend must not take a gesture lock")
	}
	close(release)
}

// cancelOnDownBackend cancels the caller ctx from inside MouseDown and records
// whether each op saw a live context — reproduces "caller gave up mid-gesture".
type cancelOnDownBackend struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	ops    []string
	alive  map[string]bool
}

func newCancelOnDownBackend() *cancelOnDownBackend {
	return &cancelOnDownBackend{alive: map[string]bool{}}
}

func (c *cancelOnDownBackend) log(ctx context.Context, op string) {
	c.mu.Lock()
	c.ops = append(c.ops, op)
	c.alive[op] = ctx.Err() == nil
	c.mu.Unlock()
}

func (c *cancelOnDownBackend) MouseMove(ctx context.Context, _, _ float64) error {
	c.log(ctx, "move")
	return nil
}

func (c *cancelOnDownBackend) MouseDown(ctx context.Context, _, _ float64, _ MouseButton) error {
	c.log(ctx, "down")
	c.cancel()
	return nil
}

func (c *cancelOnDownBackend) MouseUp(ctx context.Context, _, _ float64, _ MouseButton) error {
	c.log(ctx, "up")
	return nil
}

func (c *cancelOnDownBackend) MouseScroll(context.Context, float64, float64, float64, float64) error {
	return nil
}
func (c *cancelOnDownBackend) KeyChar(context.Context, rune) error              { return nil }
func (c *cancelOnDownBackend) InsertText(context.Context, string) error         { return nil }
func (c *cancelOnDownBackend) KeyCombo(context.Context, uint16, Modifier) error { return nil }

// TestTap_ReleasesButtonAfterCallerCtxCancelled: Tap must not release on the
// dead caller ctx — a BiDi/native release that gets dropped leaves the button
// physically held down in Firefox.
func TestTap_ReleasesButtonAfterCallerCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := newCancelOnDownBackend()
	b.cancel = cancel

	_ = NewTouchScreen(b).Tap(ctx, 5, 6)

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ops) == 0 || b.ops[len(b.ops)-1] != "up" {
		t.Fatalf("Tap must still release the button, ops=%v", b.ops)
	}
	if !b.alive["up"] {
		t.Fatal("Tap released on the cancelled caller ctx — the release would be dropped")
	}
}

// TestReleaseCtx_PassesLiveCtxThrough keeps the happy path allocation-free and
// deadline-preserving; only a dead ctx is swapped for the background one.
func TestReleaseCtx_PassesLiveCtxThrough(t *testing.T) {
	ctx := context.Background()
	got, cancel := ReleaseCtx(ctx)
	defer cancel()
	if got != ctx {
		t.Fatal("live ctx must pass through unchanged")
	}

	dead, dcancel := context.WithCancel(context.Background())
	dcancel()
	got, cancel = ReleaseCtx(dead)
	defer cancel()
	if got.Err() != nil {
		t.Fatal("dead ctx must be swapped for a live release ctx")
	}
	if _, ok := got.Deadline(); !ok {
		t.Fatal("fallback release ctx must be bounded")
	}
}

// SetWindow is unused by this fake — the frame lives in the real native backend.
func (*gestureFake) SetWindow(Window) {}

// SetWindow is unused by this fake — the frame lives in the real native backend.
func (*cancelOnDownBackend) SetWindow(Window) {}
