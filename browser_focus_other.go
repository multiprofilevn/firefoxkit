//go:build !windows

package firefoxkit

import (
	"context"

	"github.com/tuwibu/firefoxkit/input"
)

// HWND is Windows-only. Stub returns 0.
func (b *Browser) HWND() (uintptr, error) { return 0, ErrNativeNotSupported }

// FindHWND is Windows-only.
func FindHWND(pid int) (uintptr, error) { return 0, ErrNativeNotSupported }

// Focus is Windows-only. The ctx parameter mirrors chromekit's signature.
func (b *Browser) Focus(ctx context.Context) error { return ErrNativeNotSupported }

// ContentOffset is Windows-only.
func (b *Browser) ContentOffset() (int, int, error) { return 0, 0, ErrNativeNotSupported }

// WindowScale is Windows-only.
func (b *Browser) WindowScale() (float64, error) { return 0, ErrNativeNotSupported }

// WindowRect is Windows-only.
func (b *Browser) WindowRect() (input.Rect, error) { return input.Rect{}, ErrNativeNotSupported }

// InvalidateNativeWindow is Windows-only — there is no native coordinate frame
// to re-measure.
func (b *Browser) InvalidateNativeWindow() error { return ErrNativeNotSupported }

// SetWindowBounds is Windows-only.
func (b *Browser) SetWindowBounds(x, y, w, h int) error { return ErrNativeNotSupported }
