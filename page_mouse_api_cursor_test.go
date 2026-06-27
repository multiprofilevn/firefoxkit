package firefoxkit

import (
	"context"
	"strings"
	"testing"

	"github.com/tuwibu/firefoxkit/input"
)

// fakeCursorBackend implements input.Backend and records mouse opcodes so we
// can assert MouseAPI routes through the Cursor bezier engine (glide-then-press)
// instead of dispatching a single teleport-click straight to the backend.
type fakeCursorBackend struct {
	ops   []string // "move" | "down" | "up" | "scroll"
	lastX float64
	lastY float64
}

func (f *fakeCursorBackend) MouseMove(_ context.Context, x, y float64) error {
	f.ops = append(f.ops, "move")
	f.lastX, f.lastY = x, y
	return nil
}
func (f *fakeCursorBackend) MouseDown(_ context.Context, x, y float64, _ input.MouseButton) error {
	f.ops = append(f.ops, "down")
	f.lastX, f.lastY = x, y
	return nil
}
func (f *fakeCursorBackend) MouseUp(_ context.Context, x, y float64, _ input.MouseButton) error {
	f.ops = append(f.ops, "up")
	f.lastX, f.lastY = x, y
	return nil
}
func (f *fakeCursorBackend) MouseScroll(_ context.Context, _, _, _, _ float64) error {
	f.ops = append(f.ops, "scroll")
	return nil
}
func (f *fakeCursorBackend) KeyChar(_ context.Context, _ rune) error      { return nil }
func (f *fakeCursorBackend) InsertText(_ context.Context, _ string) error { return nil }
func (f *fakeCursorBackend) KeyCombo(_ context.Context, _ uint16, _ input.Modifier) error {
	return nil
}

// movesBeforeFirst returns how many "move" ops precede the first occurrence of
// op (e.g. "down"), and whether op was found at all.
func (f *fakeCursorBackend) movesBeforeFirst(op string) (int, bool) {
	n := 0
	for _, o := range f.ops {
		if o == op {
			return n, true
		}
		if o == "move" {
			n++
		}
	}
	return n, false
}

func newCursorTestPage(profile input.Profile) (*Page, *fakeCursorBackend) {
	p := &Page{browser: newTestBrowser()}
	fb := &fakeCursorBackend{}
	// Inject the fake backend on both seams: cursorEngine() honors the pre-set
	// p.cursor, and inputBackend() honors the pre-set p.bkCache.bk (used by the
	// drag press-hold portion). Both must point at the SAME fake so the op log
	// is unified and no real BiDi/native backend is built.
	p.bkCache.bk = fb
	p.cursor = input.NewCursorWithProfile(fb, profile)
	return p, fb
}

// TestMouseClickAt_GlidesThroughCursor: ProfileHuman ClickAt must emit a
// multi-step bezier path (≥2 moves) BEFORE the press, ending at the target.
func TestMouseClickAt_GlidesThroughCursor(t *testing.T) {
	p, fb := newCursorTestPage(input.ProfileHuman)
	if err := p.Mouse().ClickAt(100, 200); err != nil {
		t.Fatalf("ClickAt: %v", err)
	}
	moves, ok := fb.movesBeforeFirst("down")
	if !ok {
		t.Fatalf("no MouseDown emitted; ops=%v", fb.ops)
	}
	if moves < 2 {
		t.Fatalf("want ≥2 glide moves before press (bezier), got %d; ops=%v", moves, fb.ops)
	}
	if fb.lastX != 100 || fb.lastY != 200 {
		t.Fatalf("press should land at (100,200), got (%g,%g)", fb.lastX, fb.lastY)
	}
	if got := strings.Join(fb.ops[len(fb.ops)-2:], ","); got != "down,up" {
		t.Fatalf("want trailing down,up, got %q", got)
	}
}

// TestMouseClickAt_ProfileMachine_OneShot proves cursorProfile is consumed:
// ProfileMachine performs exactly one move then press/release.
func TestMouseClickAt_ProfileMachine_OneShot(t *testing.T) {
	p, fb := newCursorTestPage(input.ProfileMachine)
	if err := p.Mouse().ClickAt(50, 60); err != nil {
		t.Fatalf("ClickAt: %v", err)
	}
	moves, _ := fb.movesBeforeFirst("down")
	if moves != 1 {
		t.Fatalf("ProfileMachine: want exactly 1 move before press, got %d; ops=%v", moves, fb.ops)
	}
}

// TestMiddleClick_RoutesThroughCursor: the Firefox-extra coord click also glides.
func TestMiddleClick_RoutesThroughCursor(t *testing.T) {
	p, fb := newCursorTestPage(input.ProfileHuman)
	if err := p.Mouse().MiddleClick(10, 20); err != nil {
		t.Fatalf("MiddleClick: %v", err)
	}
	if moves, ok := fb.movesBeforeFirst("down"); !ok || moves < 2 {
		t.Fatalf("MiddleClick should glide ≥2 moves, got %d ok=%v; ops=%v", moves, ok, fb.ops)
	}
}

// TestMoveThenClick_SeedsFromLast: a second move starts from where the first
// ended (shared Cursor state), not from (0,0).
func TestMoveThenClick_SeedsFromLast(t *testing.T) {
	p, fb := newCursorTestPage(input.ProfileHuman)
	if err := p.Mouse().MoveTo(300, 300); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	firstMoves := len(fb.ops)
	if err := p.Mouse().ClickAt(305, 305); err != nil {
		t.Fatalf("ClickAt: %v", err)
	}
	// Bezier from (300,300) to (305,305) is short → few steps; from (0,0) it
	// would be long → many steps. Assert the second leg is short (seeded).
	secondLegMoves := 0
	for _, o := range fb.ops[firstMoves:] {
		if o == "move" {
			secondLegMoves++
		}
	}
	if secondLegMoves == 0 {
		t.Fatalf("second click emitted no moves; ops=%v", fb.ops)
	}
	if secondLegMoves > 8 {
		t.Fatalf("second leg too long (%d moves) — likely re-seeded from (0,0) not (300,300)", secondLegMoves)
	}
}

// TestDrag_ApproachGlides: Drag must glide to the start point before pressing,
// then move through intermediates and release.
func TestDrag_ApproachGlides(t *testing.T) {
	p, fb := newCursorTestPage(input.ProfileHuman)
	if err := p.Mouse().Drag(0, 0, 200, 200); err != nil {
		t.Fatalf("Drag: %v", err)
	}
	if moves, ok := fb.movesBeforeFirst("down"); !ok || moves < 1 {
		t.Fatalf("Drag should glide to start before press, got %d ok=%v; ops=%v", moves, ok, fb.ops)
	}
	if fb.ops[len(fb.ops)-1] != "up" {
		t.Fatalf("Drag should end with release, ops=%v", fb.ops)
	}
}

// TestMoveAfterDrag_SeedsFromDragEnd: after a Drag the cursor engine must be
// synced to the drag end-point, so a following MoveTo glides from there (short
// path) — not from the drag source / origin (long path). Guards the BiDi
// backend, which has no OSPositionCSS to re-seed from.
func TestMoveAfterDrag_SeedsFromDragEnd(t *testing.T) {
	p, fb := newCursorTestPage(input.ProfileHuman)
	if err := p.Mouse().Drag(0, 0, 400, 400); err != nil {
		t.Fatalf("Drag: %v", err)
	}
	preMoves := len(fb.ops)
	if err := p.Mouse().MoveTo(405, 405); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	legMoves := 0
	for _, o := range fb.ops[preMoves:] {
		if o == "move" {
			legMoves++
		}
	}
	if legMoves == 0 {
		t.Fatalf("MoveTo after drag emitted no moves; ops=%v", fb.ops)
	}
	if legMoves > 8 {
		t.Fatalf("MoveTo after drag too long (%d moves) — seeded from origin not drag end (400,400)", legMoves)
	}
}
