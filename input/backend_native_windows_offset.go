//go:build windows

package input

import (
	"fmt"
	"syscall"
	"unsafe"
)

// MozillaWindowClass is the Win32 window class that Firefox uses for its
// top-level browser windows. Differs from Chrome's "Chrome_WidgetWin_1".
const MozillaWindowClass = "MozillaWindowClass"

// Default content-area offsets for Firefox (no theme, default toolbars).
// Measured on Firefox 130 ESR Windows 11 100% DPI:
//   - Title bar:                ~30 px
//   - Tab strip:                ~36 px
//   - URL toolbar:              ~38 px
//   - Bookmarks bar (off):      0 px
//
// Total Y offset ~104 px. X offset is the window border (~8 px on Win11).
//
// These are coarse defaults; callers needing pixel accuracy should pass
// WithNativeWindow(...) to override after calibration.
const (
	DefaultFirefoxContentOffsetX = 8
	DefaultFirefoxContentOffsetY = 104
)

var (
	offsetUser32        = syscall.NewLazyDLL("user32.dll")
	procGetClientRect   = offsetUser32.NewProc("GetClientRect")
	procClientToScreen  = offsetUser32.NewProc("ClientToScreen")
	procGetDpiForWindow = offsetUser32.NewProc("GetDpiForWindow")
	procGetWindowRect   = offsetUser32.NewProc("GetWindowRect")
)

// Seams over the Win32 measurements. Production points them at the real
// syscalls; tests replace them so client geometry, DPI and window geometry are
// inputs rather than properties of whatever machine happens to run the suite.
var (
	clientOrigin  = rawClientOrigin
	dpiForWindow  = rawDpiForWindow
	windowRectFor = rawWindowRect
)

// MeasureClientOrigin returns the screen position of the window's CLIENT area
// top-left, straight from GetClientRect + ClientToScreen.
//
// This is the trusted base for Firefox's content offset: unlike Chrome — which
// paints its own title bar inside the Win32 client area, making the client rect
// useless — Firefox's client origin is a real window-manager number that page
// script cannot influence. Whatever correction is layered on top of it (the
// chrome-area height) must never be allowed to replace it.
func MeasureClientOrigin(hwnd uintptr) (x, y int, err error) {
	return clientOrigin(hwnd)
}

// MeasureContentOffset returns the (x, y) screen pixel where the page content's
// (0, 0) lives: the client origin plus the chrome-area height.
//
// The chrome-area term is the coarse DefaultFirefoxContentOffsetY constant.
// Browser.ContentOffset refines it with a measured correction; this function is
// the no-browser fallback.
func MeasureContentOffset(hwnd uintptr) (x, y int, err error) {
	cx, cy, err := MeasureClientOrigin(hwnd)
	if err != nil {
		return 0, 0, err
	}
	return cx, cy + DefaultFirefoxContentOffsetY, nil
}

// MeasureDPIScale returns the per-monitor DPI scale factor for the window
// (1.0 at 96 DPI, 1.25 at 120, 1.5 at 144). Used so CSS pixels map to the right
// number of physical screen pixels under display scaling.
//
// The kit measures this itself rather than accepting it from the caller: a
// scale the caller decided up front and the scale the window is actually
// rendered at are two different numbers the moment a window is dragged to
// another monitor.
//
// Falls back to 1.0 when GetDpiForWindow is unavailable (pre-Windows 10 1607)
// or reports 0.
func MeasureDPIScale(hwnd uintptr) float64 {
	dpi, ok := dpiForWindow(hwnd)
	if !ok || dpi == 0 {
		return 1.0
	}
	return float64(dpi) / 96.0
}

// MeasureWindowRect returns the window's outer rectangle in absolute screen
// pixels. Out-of-band by construction — GetWindowRect answers to the window
// manager, not to anything running inside the page — which is what lets it
// bound an offset measured by page script.
func MeasureWindowRect(hwnd uintptr) (Rect, error) {
	return windowRectFor(hwnd)
}

func rawClientOrigin(hwnd uintptr) (int, int, error) {
	type rect struct{ Left, Top, Right, Bottom int32 }
	type point struct{ X, Y int32 }

	var r rect
	if ret, _, e := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return 0, 0, fmt.Errorf("GetClientRect: %v", e)
	}
	pt := point{X: r.Left, Y: r.Top}
	if ret, _, e := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt))); ret == 0 {
		return 0, 0, fmt.Errorf("ClientToScreen: %v", e)
	}
	return int(pt.X), int(pt.Y), nil
}

func rawDpiForWindow(hwnd uintptr) (uint32, bool) {
	if err := procGetDpiForWindow.Find(); err != nil {
		return 0, false
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	return uint32(dpi), true
}

func rawWindowRect(hwnd uintptr) (Rect, error) {
	var r struct{ Left, Top, Right, Bottom int32 }
	ret, _, e := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return Rect{}, fmt.Errorf("GetWindowRect: %v", e)
	}
	return Rect{Left: int(r.Left), Top: int(r.Top), Right: int(r.Right), Bottom: int(r.Bottom)}, nil
}
