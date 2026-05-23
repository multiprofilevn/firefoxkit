//go:build !windows

package firefoxkit

// HWND is Windows-only. Stub returns 0.
func (b *Browser) HWND() (uintptr, error) { return 0, ErrNativeNotSupported }

// FocusWindow is Windows-only.
func (b *Browser) FocusWindow() error { return ErrNativeNotSupported }
