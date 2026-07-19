package input

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countBackend is a goroutine-safe Backend stub counting mouse calls.
type countBackend struct {
	moveCalls atomic.Int32
}

func (c *countBackend) MouseMove(context.Context, float64, float64) error {
	c.moveCalls.Add(1)
	return nil
}
func (c *countBackend) MouseDown(context.Context, float64, float64, MouseButton) error { return nil }
func (c *countBackend) MouseUp(context.Context, float64, float64, MouseButton) error   { return nil }
func (c *countBackend) MouseScroll(context.Context, float64, float64, float64, float64) error {
	return nil
}
func (c *countBackend) KeyChar(context.Context, rune) error        { return nil }
func (c *countBackend) InsertText(context.Context, string) error   { return nil }
func (c *countBackend) KeyCombo(context.Context, uint16, Modifier) error { return nil }

// batchBackend records MousePath calls and asserts they never overlap.
type batchBackend struct {
	countBackend
	mu       sync.Mutex
	batches  [][]PathPoint
	inFlight atomic.Int32
	overlap  atomic.Bool
}

func (b *batchBackend) MousePath(_ context.Context, pts []PathPoint) error {
	if b.inFlight.Add(1) > 1 {
		b.overlap.Store(true)
	}
	time.Sleep(5 * time.Millisecond) // widen the overlap window
	cp := make([]PathPoint, len(pts))
	copy(cp, pts)
	b.mu.Lock()
	b.batches = append(b.batches, cp)
	b.mu.Unlock()
	b.inFlight.Add(-1)
	return nil
}

func (b *batchBackend) allPoints() []PathPoint {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []PathPoint
	for _, bt := range b.batches {
		out = append(out, bt...)
	}
	return out
}

// remotePathServer serves n points ending at (toX, toY) with the given delay.
func remotePathServer(t *testing.T, n int, toX, toY, delay float64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pts := make([]map[string]any, n)
		for i := range pts {
			pts[i] = map[string]any{"x": float64(i), "y": float64(i), "delay": delay}
		}
		pts[n-1] = map[string]any{"x": toX, "y": toY, "delay": delay}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": pts})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestMoveTo_Batched_SingleCallNotPerPoint locks in M9: a remote path of 80
// points must reach a batching backend as one MousePath call — not 80
// MouseMove round-trips.
func TestMoveTo_Batched_SingleCallNotPerPoint(t *testing.T) {
	srv := remotePathServer(t, 80, 100, 100, 5)
	bk := &batchBackend{}
	c := NewCursorWithProfileAndPath(bk, ProfileHuman, srv.URL)

	if err := c.MoveTo(context.Background(), 100, 100); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if len(bk.batches) != 1 {
		t.Fatalf("MousePath calls = %d, want 1", len(bk.batches))
	}
	if got := len(bk.batches[0]); got != 80 {
		t.Fatalf("batched points = %d, want 80", got)
	}
	if bk.moveCalls.Load() != 0 {
		t.Fatalf("MouseMove called %d times — path must be batched", bk.moveCalls.Load())
	}
}

// TestRemotePath_DelayCapped locks in M6: server delays are clamped so a bogus
// delay (60s/point) can't wedge a move for minutes.
func TestRemotePath_DelayCapped(t *testing.T) {
	srv := remotePathServer(t, 10, 50, 50, 60000)
	bk := &batchBackend{}
	c := NewCursorWithProfileAndPath(bk, ProfileHuman, srv.URL)

	if err := c.MoveTo(context.Background(), 50, 50); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	for _, p := range bk.allPoints() {
		if p.DelayMs > maxRemoteDelayMs {
			t.Fatalf("delay %dms exceeds cap %dms", p.DelayMs, maxRemoteDelayMs)
		}
	}
}

// TestRemotePath_TooManyPoints_FallsBack: a server path over the point cap is
// rejected in favor of the local bezier (which ends at the same target).
func TestRemotePath_TooManyPoints_FallsBack(t *testing.T) {
	srv := remotePathServer(t, maxRemotePathPoints+500, 60, 60, 1)
	bk := &batchBackend{}
	c := NewCursorWithProfileAndPath(bk, ProfileHuman, srv.URL)

	if err := c.MoveTo(context.Background(), 60, 60); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	pts := bk.allPoints()
	if len(pts) == 0 || len(pts) > maxRemotePathPoints {
		t.Fatalf("expected bezier fallback path, got %d points", len(pts))
	}
	last := pts[len(pts)-1]
	if last.X != 60 || last.Y != 60 {
		t.Fatalf("fallback path ends at (%v,%v), want (60,60)", last.X, last.Y)
	}
}

// TestMoveTo_Concurrent_NoInterleave locks in M4/M9: concurrent MoveTo calls
// on one cursor must serialize — their paths never overlap in flight.
func TestMoveTo_Concurrent_NoInterleave(t *testing.T) {
	bk := &batchBackend{}
	c := NewCursorWithProfile(bk, ProfileHuman)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = c.MoveTo(context.Background(), float64(100+i*10), float64(50+i*10))
		}(i)
	}
	wg.Wait()

	if bk.overlap.Load() {
		t.Fatal("MousePath calls overlapped — cursor mutex not serializing moves")
	}
	if len(bk.batches) != 4 {
		t.Fatalf("MousePath calls = %d, want 4", len(bk.batches))
	}
}

// TestMoveTo_NonBatchingBackend_PerPointFallback: the native backend (no
// PathBatcher) still receives per-point MouseMove calls.
func TestMoveTo_NonBatchingBackend_PerPointFallback(t *testing.T) {
	bk := &countBackend{}
	c := NewCursorWithProfile(bk, ProfileFast)

	if err := c.MoveTo(context.Background(), 40, 40); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if bk.moveCalls.Load() == 0 {
		t.Fatal("expected per-point MouseMove calls on non-batching backend")
	}
}
