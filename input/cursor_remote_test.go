package input

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidRemotePath_RejectsNonFiniteInterior(t *testing.T) {
	pts := []remotePoint{
		{X: 0, Y: 0},
		{X: math.NaN(), Y: 5},
		{X: 300, Y: 200},
	}
	if validRemotePath(pts, 300, 200) {
		t.Fatal("path with NaN interior point must be rejected")
	}
	inf := []remotePoint{{X: 0, Y: 0}, {X: math.Inf(1), Y: 5}, {X: 300, Y: 200}}
	if validRemotePath(inf, 300, 200) {
		t.Fatal("path with Inf interior point must be rejected")
	}
}

// recordingBackend records every MouseMove point. No cursorSeeder → MoveTo uses
// the Cursor's cached position.
type recordingBackend struct{ moves []Point }

func (b *recordingBackend) MouseMove(_ context.Context, x, y float64) error {
	b.moves = append(b.moves, Point{X: x, Y: y})
	return nil
}
func (b *recordingBackend) MouseDown(_ context.Context, _, _ float64, _ MouseButton) error { return nil }
func (b *recordingBackend) MouseUp(_ context.Context, _, _ float64, _ MouseButton) error   { return nil }
func (b *recordingBackend) MouseScroll(_ context.Context, _, _, _, _ float64) error        { return nil }
func (b *recordingBackend) KeyChar(_ context.Context, _ rune) error                        { return nil }
func (b *recordingBackend) InsertText(_ context.Context, _ string) error                   { return nil }
func (b *recordingBackend) KeyCombo(_ context.Context, _ uint16, _ Modifier) error         { return nil }

func mouseServer(t *testing.T, extraLastOffset float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mouseReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		data := []remotePoint{
			{X: req.From.X, Y: req.From.Y, Delay: 0},
			{X: (req.From.X + req.To.X) / 2, Y: (req.From.Y + req.To.Y) / 2, Delay: 0},
			{X: req.To.X + extraLastOffset, Y: req.To.Y + extraLastOffset, Delay: 0},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mouseResp{Success: true, Data: data})
	}))
}

func TestMoveTo_RemotePath_ReplayedEndsOnTarget(t *testing.T) {
	srv := mouseServer(t, 0)
	defer srv.Close()

	fb := &recordingBackend{}
	c := NewCursorWithProfileAndPath(fb, ProfileHuman, srv.URL)
	if err := c.MoveTo(context.Background(), 300, 200); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if len(fb.moves) != 3 {
		t.Fatalf("expected 3 replayed moves from server, got %d", len(fb.moves))
	}
	last := fb.moves[len(fb.moves)-1]
	if last.X != 300 || last.Y != 200 {
		t.Fatalf("last move must land on target (300,200), got (%v,%v)", last.X, last.Y)
	}
}

func TestMoveTo_EmptyURL_UsesLocalBezier(t *testing.T) {
	fb := &recordingBackend{}
	c := NewCursorWithProfileAndPath(fb, ProfileHuman, "")
	if err := c.MoveTo(context.Background(), 300, 200); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if len(fb.moves) < 8 {
		t.Fatalf("empty URL should use local bezier (>=8 steps), got %d", len(fb.moves))
	}
	last := fb.moves[len(fb.moves)-1]
	if last.X != 300 || last.Y != 200 {
		t.Fatalf("local bezier last point must be target, got (%v,%v)", last.X, last.Y)
	}
}

func TestMoveTo_ServerError_FallsBackToLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fb := &recordingBackend{}
	c := NewCursorWithProfileAndPath(fb, ProfileHuman, srv.URL)
	if err := c.MoveTo(context.Background(), 300, 200); err != nil {
		t.Fatalf("MoveTo must not error on server failure (fallback), got %v", err)
	}
	if len(fb.moves) < 8 {
		t.Fatalf("server 500 should fall back to local bezier, got %d moves", len(fb.moves))
	}
	last := fb.moves[len(fb.moves)-1]
	if last.X != 300 || last.Y != 200 {
		t.Fatalf("fallback last point must be target, got (%v,%v)", last.X, last.Y)
	}
}

func TestMoveTo_OffTargetRemotePath_FallsBackToLocal(t *testing.T) {
	srv := mouseServer(t, 50)
	defer srv.Close()

	fb := &recordingBackend{}
	c := NewCursorWithProfileAndPath(fb, ProfileHuman, srv.URL)
	_ = c.MoveTo(context.Background(), 300, 200)
	if len(fb.moves) < 8 {
		t.Fatalf("off-target remote path should fall back to local, got %d moves", len(fb.moves))
	}
	last := fb.moves[len(fb.moves)-1]
	if last.X != 300 || last.Y != 200 {
		t.Fatalf("fallback last point must be exact target, got (%v,%v)", last.X, last.Y)
	}
}
