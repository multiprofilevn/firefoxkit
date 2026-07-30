//go:build windows

package firefoxkit

import (
	"context"
	"fmt"
	"syscall"
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
