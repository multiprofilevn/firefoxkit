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
// Total Y offset ~104 px. X offset is the window border (~8 px on Win11).
//
// These are coarse defaults; callers needing pixel accuracy should pass
// WithNativeWindow(...) to override after calibration.
const (
	DefaultFirefoxContentOffsetX = 8
	DefaultFirefoxContentOffsetY = 104
)

// MeasureContentOffset queries the window's client rectangle and returns the
// (x, y) screen pixel where the page content's (0, 0) lives. Subtract the
// window's top-left from this to get the relative offset stored in
// Window.ContentOffsetX/Y.
//
// Firefox doesn't expose a stable API for "where does content start in the
// chrome area" — the value depends on theme + tab strip + URL bar visibility.
// We approximate via GetClientRect + ClientToScreen, then add the empirical
// chrome-area constants above. Callers that need exactness can pass an
// override via WithNativeWindow(x, y, scale).
func MeasureContentOffset(hwnd uintptr) (x, y int, err error) {
	type rect struct{ Left, Top, Right, Bottom int32 }
	type point struct{ X, Y int32 }

	user32 := syscall.NewLazyDLL("user32.dll")
	getClientRect := user32.NewProc("GetClientRect")
	clientToScreen := user32.NewProc("ClientToScreen")

	var r rect
	if ret, _, e := getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return 0, 0, fmt.Errorf("GetClientRect: %v", e)
	}
	pt := point{X: r.Left, Y: r.Top}
	if ret, _, e := clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt))); ret == 0 {
		return 0, 0, fmt.Errorf("ClientToScreen: %v", e)
	}
	// pt is now content-area top-left in screen coords; add chrome-area Y.
	return int(pt.X), int(pt.Y) + DefaultFirefoxContentOffsetY, nil
}
