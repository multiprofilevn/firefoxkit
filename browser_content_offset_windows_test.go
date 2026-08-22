//go:build windows

package firefoxkit

import (
	"strings"
	"sync"
	"testing"

	"github.com/multiprofilevn/firefoxkit/input"
)

// Firefox does NOT paint its chrome inside the Win32 client area, so
// GetClientRect + ClientToScreen give a real, page-unreachable answer for where
// the client area starts. That base stays authoritative. The only thing a
// page-side measurement is allowed to refine is the chrome-area height on top
// of it — and only within a sane range.
//
// Porting Chrome's all-JS measurement here "for symmetry" would trade a trusted
// number for an attacker-controlled one to win about twenty pixels. The
// symmetry that matters is the SEMANTICS of the return value, not the syscall
// used to produce it.

// captureLogger records Warnf calls so a spec can assert that a discarded
// measurement was reported rather than silently swallowed.
type captureLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *captureLogger) Debugf(string, ...any) {}
func (l *captureLogger) Infof(string, ...any)  {}
func (l *captureLogger) Errorf(string, ...any) {}
func (l *captureLogger) Warnf(f string, args ...any) {
	l.mu.Lock()
	l.warns = append(l.warns, f)
	l.mu.Unlock()
}
func (l *captureLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.warns)
}
func (l *captureLogger) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.warns, "\n")
}

// newOffsetTestBrowser wires both measurement seams to fixtures and returns a
// browser whose HWND resolves without a real Firefox process.
func newOffsetTestBrowser(t *testing.T, originX, originY int, correction int, haveCorrection bool) (*Browser, *captureLogger, *int) {
	t.Helper()
	prevOrigin, prevCorr := clientOriginOf, chromeAreaCorrection
	t.Cleanup(func() { clientOriginOf, chromeAreaCorrection = prevOrigin, prevCorr })

	originCalls := 0
	clientOriginOf = func(uintptr) (int, int, error) {
		originCalls++
		return originX, originY, nil
	}
	chromeAreaCorrection = func(*Browser) (int, bool) { return correction, haveCorrection }

	log := &captureLogger{}
	b := newTestBrowser()
	b.log = log
	b.hwndOverride = 0x1234
	return b, log, &originCalls
}

// 1. TestContentOffset_IsAbsolute: firefoxkit's ContentOffset already returns an
// absolute screen position, and must keep doing so once chromekit is brought in
// line. Two different WithNativeWindow declarations, one answer.
func TestContentOffset_IsAbsolute(t *testing.T) {
	b1, _, _ := newOffsetTestBrowser(t, 100, 300, 40, true)
	b1.cfg = defaultConfig()
	WithNativeWindow(0, 0, 1.0)(b1.cfg)
	x1, y1, err := b1.ContentOffset()
	if err != nil {
		t.Fatalf("ContentOffset (declared 0,0): %v", err)
	}

	b2, _, _ := newOffsetTestBrowser(t, 100, 300, 40, true)
	b2.cfg = defaultConfig()
	WithNativeWindow(100, 300, 1.0)(b2.cfg)
	x2, y2, err := b2.ContentOffset()
	if err != nil {
		t.Fatalf("ContentOffset (declared 100,300): %v", err)
	}

	if x1 != x2 || y1 != y2 {
		t.Fatalf("ContentOffset depends on the declared window position: (%d,%d) vs (%d,%d) — it must be absolute",
			x1, y1, x2, y2)
	}
}

// 18. TestFirefoxContentOffset_UsesWin32Base: the Win32 client origin is the
// base, and the measured chrome-area height is a correction layered on top of
// it — not a replacement for it.
//
// The "base was consulted" half is the anti-regression half: it fails loudly if
// someone later swaps the whole measurement over to script.evaluate.
func TestFirefoxContentOffset_UsesWin32Base(t *testing.T) {
	b, _, originCalls := newOffsetTestBrowser(t, 100, 300, 40, true)

	gotX, gotY, err := b.ContentOffset()
	if err != nil {
		t.Fatalf("ContentOffset: %v", err)
	}
	if *originCalls == 0 {
		t.Fatal("ContentOffset never consulted the Win32 client origin — the trusted base has been abandoned")
	}
	if gotX != 100 {
		t.Fatalf("ContentOffset x = %d, want the client origin x 100 — the correction must not move x", gotX)
	}
	if gotY != 340 {
		t.Fatalf("ContentOffset y = %d, want 340 (client origin 300 + measured chrome area 40); "+
			"%d means the coarse %d constant is still in use and the measurement was ignored",
			gotY, 300+input.DefaultFirefoxContentOffsetY, input.DefaultFirefoxContentOffsetY)
	}
}

// 12. TestFirefoxContentOffset_ClampsChromeAreaCorrection: a correction of 900px
// is not a tall toolbar, it is a broken (or hostile) measurement. Discard it,
// fall back to the constant, and say so — a silent fallback turns a measurement
// bug into a permanent 20px click offset nobody can trace.
func TestFirefoxContentOffset_ClampsChromeAreaCorrection(t *testing.T) {
	b, log, _ := newOffsetTestBrowser(t, 100, 300, 900, true)

	gotX, gotY, err := b.ContentOffset()
	if err != nil {
		t.Fatalf("ContentOffset: %v", err)
	}
	wantY := 300 + input.DefaultFirefoxContentOffsetY
	if gotX != 100 || gotY != wantY {
		t.Fatalf("ContentOffset = (%d,%d), want (100,%d) — an out-of-range correction must be discarded, not applied",
			gotX, gotY, wantY)
	}
	if log.warnCount() == 0 {
		t.Fatalf("a chrome-area correction of 900px (outside 0-%d) was discarded silently; it must be logged at Warn",
			chromeAreaCorrectionMax)
	}
	if !strings.Contains(strings.ToLower(log.joined()), "chrome") {
		t.Fatalf("Warn text %q does not mention what was discarded", log.joined())
	}
}
