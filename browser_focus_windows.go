//go:build windows

package firefoxkit

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tuwibu/firefoxkit/input"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
)

const swRestore = 9

// SetWindowPos flags: keep Z-order and don't steal focus when repositioning.
const (
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
)

// Per-process state for findFirefoxHWND. The callback is allocated EXACTLY
// ONCE via syscall.NewCallback (Go's callback table is a fixed-size slab
// that does NOT get garbage-collected — allocating per call exhausts it
// after ~1024 lookups). Thread the pid/match through package vars under
// findHwndMu so the single callback closure has nothing to capture.
var (
	findHwndOnce sync.Once
	findHwndCB   uintptr
	findHwndMu   sync.Mutex
	findHwndPID  uint32
	findHwndOut  uintptr
)

func findHwndCallback(hwnd, _ uintptr) uintptr {
	var winPID uint32
	_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&winPID)))
	if winPID != findHwndPID {
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
	findHwndPID = pid
	findHwndOut = 0
	_, _, _ = procEnumWindows.Call(findHwndCB, 0)
	return findHwndOut, nil
}

// Focus brings the Firefox window to the foreground. Restores from minimized
// state if needed. Required before native input can hit content. The ctx
// parameter mirrors chromekit's signature; the win32 foreground calls are
// synchronous so it is currently only honored for cancellation pre-check.
func (b *Browser) Focus(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	hwnd, err := b.HWND()
	if err != nil {
		return err
	}
	if hwnd == 0 {
		return fmt.Errorf("firefoxkit: Firefox HWND not found")
	}
	if ret, _, _ := procIsIconic.Call(hwnd); ret != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
	}
	if ret, _, e := procSetForegroundWindow.Call(hwnd); ret == 0 {
		return fmt.Errorf("SetForegroundWindow: %v", e)
	}
	return nil
}

// ContentOffset returns the (x, y) pixel offset from the Firefox window's
// top-left to the top-left of the rendered web content — the chrome height
// plus border. Used to translate page coordinates to screen coordinates for
// native input.
func (b *Browser) ContentOffset() (int, int, error) {
	hwnd, err := b.HWND()
	if err != nil {
		return 0, 0, err
	}
	return input.MeasureContentOffset(hwnd)
}

// WindowScale returns the per-monitor DPI scale factor of the Firefox window
// (1.0 at 96 DPI). Used by the native input backend to map CSS pixels to
// physical screen pixels under display scaling.
func (b *Browser) WindowScale() (float64, error) {
	hwnd, err := b.HWND()
	if err != nil {
		return 0, err
	}
	return input.MeasureDPIScale(hwnd), nil
}

// SetWindowBounds positions and sizes the Firefox window in screen pixels.
// Firefox has no --window-position CLI flag, so native-input callers running
// multiple profiles use this to tile windows into a non-overlapping grid (a
// shared OS cursor sends clicks to whichever window sits under the point).
// Z-order and focus are preserved.
func (b *Browser) SetWindowBounds(x, y, w, h int) error {
	hwnd, err := b.HWND()
	if err != nil {
		return err
	}
	if hwnd == 0 {
		return fmt.Errorf("firefoxkit: Firefox HWND not found")
	}
	ret, _, e := procSetWindowPos.Call(
		hwnd, 0,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(swpNoZOrder|swpNoActivate),
	)
	if ret == 0 {
		return fmt.Errorf("SetWindowPos: %v", e)
	}
	return nil
}
