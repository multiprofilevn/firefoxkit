//go:build windows

package firefoxkit

// Resolution of the Firefox top-level window handle from a launched PID.
// Split out of browser_focus_windows.go (which now only consumes HWND()) to
// keep both files under the 200-line budget.

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tuwibu/firefoxkit/input"
)

// Per-process state for findFirefoxHWND. The callback is allocated EXACTLY
// ONCE via syscall.NewCallback (Go's callback table is a fixed-size slab
// that does NOT get garbage-collected — allocating per call exhausts it
// after ~1024 lookups). Thread the pid/match through package vars under
// findHwndMu so the single callback closure has nothing to capture.
//
// findHwndSet is the optional descendant fallback set (nil on the exact-PID
// pass); see findFirefoxHWND for why a descendant may own the window.
var (
	findHwndOnce sync.Once
	findHwndCB   uintptr
	findHwndMu   sync.Mutex
	findHwndPID  uint32
	findHwndSet  map[uint32]struct{}
	findHwndOut  uintptr
)

// findHwndAccepts reports whether a window's owning PID is one we want.
// Called only from findHwndCallback, i.e. always under findHwndMu.
func findHwndAccepts(winPID uint32) bool {
	if winPID == findHwndPID {
		return true
	}
	if findHwndSet == nil {
		return false
	}
	_, ok := findHwndSet[winPID]
	return ok
}

func findHwndCallback(hwnd, _ uintptr) uintptr {
	var winPID uint32
	_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&winPID)))
	if !findHwndAccepts(winPID) {
		return 1
	}
	if ret, _, _ := procIsWindowVisible.Call(hwnd); ret == 0 {
		return 1
	}
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return 1
	}
	if syscall.UTF16ToString(buf[:n]) == input.MozillaWindowClass {
		findHwndOut = hwnd
		return 0 // stop enumeration
	}
	return 1
}

// HWND returns the top-level Mozilla window owned by this Browser's process.
// A non-zero override set via SetHWND wins. Otherwise, when proc is nil
// (Connect path) this returns an error.
func (b *Browser) HWND() (uintptr, error) {
	if b == nil {
		return 0, fmt.Errorf("firefoxkit: nil browser")
	}
	if b.hwndOverride != 0 {
		return b.hwndOverride, nil
	}
	if b.proc == nil {
		return 0, fmt.Errorf("firefoxkit: HWND unavailable on Connect path")
	}
	return findFirefoxHWND(uint32(b.proc.Pid))
}

// findFirefoxHWND enumerates visible top-level windows, matches by PID +
// MozillaWindowClass, returns the first hit. 0 if not found.
//
// TWO PASSES, IN THIS ORDER — the ordering is the PID-reuse defence:
//  1. exact PID. Windows owned by the process we actually launched are
//     unambiguous, so they always win.
//  2. transitive descendants of that PID. Firefox uses a launcher process:
//     the PID we started re-launches itself and hands off, so on builds that
//     do this the real browser window belongs to a child/grandchild.
//     Measured: launched PID 117448, window owned by PID 164404 — pass 1
//     matched nothing on all 25 instances of a real run and every native-input
//     feature degraded to a fallback. A recycled PID that happens to look like
//     a descendant can only be reached after pass 1 has already failed.
//
// Pass 2 costs a process-table snapshot, so it runs only when pass 1 misses —
// HWND() is on the hot path for Focus/ContentOffset/the foreground guard.
// Neither pass retries or sleeps: a miss returns immediately, so a genuinely
// window-less process fails fast rather than stalling the caller.
//
// IMPORTANT: callback is allocated once via sync.Once. Calls are serialized
// through findHwndMu so pid/match state thread through package vars instead
// of a closure capture (closure capture would force a fresh NewCallback per
// invocation, exhausting Go's fixed callback table).
func findFirefoxHWND(pid uint32) (uintptr, error) {
	findHwndOnce.Do(func() {
		findHwndCB = syscall.NewCallback(findHwndCallback)
	})
	findHwndMu.Lock()
	defer findHwndMu.Unlock()

	if hwnd := enumMozillaWindowLocked(pid, nil); hwnd != 0 {
		return hwnd, nil
	}

	table, err := snapshotProcessTable()
	if err != nil {
		return 0, fmt.Errorf("firefoxkit: process table snapshot for pid %d: %w", pid, err)
	}
	kin := descendantPIDs(table, pid)
	delete(kin, pid) // already covered by pass 1
	if len(kin) == 0 {
		return 0, nil
	}
	return enumMozillaWindowLocked(pid, kin), nil
}

// enumMozillaWindowLocked runs one EnumWindows pass accepting pid, plus any
// PID in extra when non-nil. Caller must hold findHwndMu.
func enumMozillaWindowLocked(pid uint32, extra map[uint32]struct{}) uintptr {
	findHwndPID = pid
	findHwndSet = extra
	findHwndOut = 0
	_, _, _ = procEnumWindows.Call(findHwndCB, 0)
	findHwndSet = nil // don't leak the set into the next caller's fast pass
	return findHwndOut
}

// FindHWND returns the top-level Mozilla window owned by pid. For callers on the
// Connect path (Browser.proc is nil, so HWND() cannot self-resolve) that learned
// the real browser PID out-of-band and need to wire it via SetHWND before native
// input. Returns an error if no matching window is found.
func FindHWND(pid int) (uintptr, error) {
	hwnd, err := findFirefoxHWND(uint32(pid))
	if err != nil {
		return 0, err
	}
	if hwnd == 0 {
		return 0, fmt.Errorf("firefoxkit: no Firefox window found for pid %d", pid)
	}
	return hwnd, nil
}
