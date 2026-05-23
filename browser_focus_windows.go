//go:build windows

package firefoxkit

import (
	"fmt"
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
)

const swRestore = 9

// HWND returns the top-level Mozilla window owned by this Browser's process.
// Connect-path callers must set b.proc via WithModifyCmd or set foreground by
// PID themselves — when proc is nil this returns 0.
func (b *Browser) HWND() (uintptr, error) {
	if b == nil || b.proc == nil {
		return 0, fmt.Errorf("firefoxkit: HWND unavailable on Connect path")
	}
	return findFirefoxHWND(uint32(b.proc.Pid))
}

// findFirefoxHWND enumerates visible top-level windows, matches by PID +
// MozillaWindowClass, returns first hit. 0 if not found. Uses a closure to
// capture pid/matched — avoids the uintptr→pointer round-trip vet warns on.
func findFirefoxHWND(pid uint32) (uintptr, error) {
	var matched uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var winPID uint32
		_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&winPID)))
		if winPID != pid {
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
		class := syscall.UTF16ToString(buf[:n])
		if class == input.MozillaWindowClass {
			matched = hwnd
			return 0 // stop
		}
		return 1
	})

	_, _, _ = procEnumWindows.Call(cb, 0)
	return matched, nil
}

// FocusWindow brings the Firefox window to the foreground. Restores from
// minimized state if needed. Required before native input can hit content.
func (b *Browser) FocusWindow() error {
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
