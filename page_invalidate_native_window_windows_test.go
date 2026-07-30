//go:build windows

package firefoxkit

import (
	"context"
	"sync"
	"testing"

	"github.com/tuwibu/firefoxkit/input"
)

// A Page caches its input backend behind THREE independent sync.Once values:
//
//	p.bkCache.once  (page_input_backend.go) — the backend itself
//	p.cursorOnce    (page_input_backend.go) — the Cursor built OVER that backend
//	p.kbOnce        (page_keyboard_api.go)  — the KeyboardAPI built OVER it too
//
// The last two capture the RESULT of p.inputBackend() at first use. Re-measuring
// the coordinate frame by rebuilding the first cache therefore fixes nothing:
// the cursor and the keyboard are still holding the object that was built with
// the stale frame, and every click keeps landing where the window used to be.
//
// The nasty part is that a test written against p.bkCache passes anyway. It
// observes the one cache that WAS refreshed and reports success while the two
// that matter are wrong. So this spec asserts at the behaviour layer only: the
// screen coordinates that actually came out of the cursor, and the frame the
// keyboard actually emitted under. Do not "simplify" it by reading bkCache.

// frameBackend is a fake Backend that models the one thing the real native
// backend does with a Window: convert CSS coordinates into absolute screen
// pixels. It records the result of that conversion, so a spec can ask "where
// would this click physically have landed" without touching the OS.
type frameBackend struct {
	mu     sync.Mutex
	win    input.Window
	moves  [][2]float64
	downs  [][2]float64
	keyWin []input.Window
}

func (f *frameBackend) SetWindow(w input.Window) {
	f.mu.Lock()
	f.win = w
	f.mu.Unlock()
}

// screen mirrors the native backend's cssToScreen for the absolute-offset
// frame: ContentOffset carries the screen position of CSS (0,0), X/Y are zero.
func (f *frameBackend) screen(x, y float64) [2]float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.win.Scale
	if s <= 0 {
		s = 1
	}
	return [2]float64{
		float64(f.win.X+f.win.ContentOffsetX) + x*s,
		float64(f.win.Y+f.win.ContentOffsetY) + y*s,
	}
}

func (f *frameBackend) MouseMove(_ context.Context, x, y float64) error {
	p := f.screen(x, y)
	f.mu.Lock()
	f.moves = append(f.moves, p)
	f.mu.Unlock()
	return nil
}

func (f *frameBackend) MouseDown(_ context.Context, x, y float64, _ input.MouseButton) error {
	p := f.screen(x, y)
	f.mu.Lock()
	f.downs = append(f.downs, p)
	f.mu.Unlock()
	return nil
}

func (f *frameBackend) MouseUp(_ context.Context, x, y float64, _ input.MouseButton) error { return nil }

func (f *frameBackend) MouseScroll(_ context.Context, _, _, _, _ float64) error { return nil }

func (f *frameBackend) KeyChar(_ context.Context, _ rune) error {
	f.mu.Lock()
	f.keyWin = append(f.keyWin, f.win)
	f.mu.Unlock()
	return nil
}

func (f *frameBackend) InsertText(_ context.Context, _ string) error { return nil }

func (f *frameBackend) KeyCombo(_ context.Context, _ uint16, _ input.Modifier) error {
	f.mu.Lock()
	f.keyWin = append(f.keyWin, f.win)
	f.mu.Unlock()
	return nil
}

func (f *frameBackend) lastMove() ([2]float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.moves) == 0 {
		return [2]float64{}, false
	}
	return f.moves[len(f.moves)-1], true
}

func (f *frameBackend) lastKeyWindow() (input.Window, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.keyWin) == 0 {
		return input.Window{}, false
	}
	return f.keyWin[len(f.keyWin)-1], true
}

// 7. TestInvalidate_FirefoxCursorAndKeyboardSeeNewWindow.
//
// Sequence matters: the cursor and the keyboard are PRIMED (their sync.Once
// fired) before the window moves. That is the real-world order — a job clicks,
// then something repositions the window, then the job clicks again — and it is
// the only order in which the stale-capture bug is reachable.
func TestInvalidate_FirefoxCursorAndKeyboardSeeNewWindow(t *testing.T) {
	prevOrigin := clientOriginOf
	t.Cleanup(func() { clientOriginOf = prevOrigin })

	// The window starts with its content origin at (100, 300); the coarse
	// chrome-area constant puts CSS (0,0) at (100, 404).
	clientOriginOf = func(uintptr) (int, int, error) { return 100, 300, nil }

	b := newTestBrowser()
	b.log = noopLogger{}
	b.cfg = defaultConfig()
	b.cfg.cursorProfile = CursorMachine // no bezier cadence; this spec is about coordinates
	b.hwndOverride = 0x1234

	fake := &frameBackend{}
	p := &Page{browser: b, contextID: "ctx-1", log: b.log}
	p.bkCache.bk = fake // honoured by inputBackend()'s "pre-set by tests" path
	b.pages = []*Page{p}
	b.current = p

	startFrame, err := frameFor(b)
	if err != nil {
		t.Fatalf("initial frame: %v", err)
	}
	fake.SetWindow(startFrame)

	// PRIME both lazy caches while the old frame is live.
	if err := p.Mouse().MoveTo(10, 10); err != nil {
		t.Fatalf("priming MoveTo: %v", err)
	}
	if err := p.Keyboard().Type("a"); err != nil {
		t.Fatalf("priming Type: %v", err)
	}

	// The window moves 800px right and 200px down.
	clientOriginOf = func(uintptr) (int, int, error) { return 900, 500, nil }
	wantFrame, err := frameFor(b)
	if err != nil {
		t.Fatalf("re-measured frame: %v", err)
	}

	if err := b.InvalidateNativeWindow(); err != nil {
		t.Fatalf("InvalidateNativeWindow: %v", err)
	}

	// --- cursor: assert the SCREEN pixel a click would physically land on ---
	if err := p.Mouse().MoveTo(100, 80); err != nil {
		t.Fatalf("post-invalidate MoveTo: %v", err)
	}
	got, ok := fake.lastMove()
	if !ok {
		t.Fatal("no cursor movement was emitted at all")
	}
	want := [2]float64{
		float64(wantFrame.ContentOffsetX) + 100*wantFrame.Scale,
		float64(wantFrame.ContentOffsetY) + 80*wantFrame.Scale,
	}
	if got != want {
		t.Fatalf("cursor emitted screen (%g,%g), want (%g,%g) — cursorEngine() captured the backend behind its own "+
			"sync.Once and is still converting through the pre-move frame",
			got[0], got[1], want[0], want[1])
	}

	// --- keyboard: assert the frame it actually emitted under ---
	if err := p.Keyboard().Type("b"); err != nil {
		t.Fatalf("post-invalidate Type: %v", err)
	}
	gotWin, ok := fake.lastKeyWindow()
	if !ok {
		t.Fatal("no keystroke was emitted at all")
	}
	if gotWin != wantFrame {
		t.Fatalf("keyboard emitted under frame %+v, want %+v — Keyboard() captured the backend behind kbOnce and is "+
			"still holding the pre-move frame", gotWin, wantFrame)
	}
}

// frameFor computes the coordinate frame the page SHOULD be using right now,
// from the same measurements the production path reads. Keeping the expectation
// derived rather than hard-coded means this spec keeps testing "the frame was
// refreshed" and not "the frame equals these six numbers".
func frameFor(b *Browser) (input.Window, error) {
	ox, oy, err := b.ContentOffset()
	if err != nil {
		return input.Window{}, err
	}
	scale, serr := b.WindowScale()
	if serr != nil || scale <= 0 {
		scale = 1.0
	}
	return buildNativeWindow(b.cfg.nativeWindow, ox, oy, scale, true), nil
}
