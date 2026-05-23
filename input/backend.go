// Package input contains the Backend interface and its implementations
// (BiDi via WebDriver BiDi performActions + Native via winapi). Kept separate
// from the root firefoxkit package so the platform code stays optional.
package input

import (
	"context"
	"errors"
)

// ErrNotSupported signals the active backend cannot perform the call on this
// platform (e.g. native backend on macOS/Linux).
var ErrNotSupported = errors.New("input: not supported on this platform")

// ErrInvalidKey is returned when a key name has no mapping in the W3C map.
var ErrInvalidKey = errors.New("input: invalid key name")

// MouseButton enumerates the buttons a backend understands.
type MouseButton int

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
)

// Type selects a backend implementation.
type Type int

const (
	TypeBiDi Type = iota
	TypeNative
)

// Window describes where the Firefox window sits on screen, so DOM coordinates
// can be converted to absolute screen pixels by the native backend.
//
// X, Y are the window's top-left in screen pixels. ContentOffsetX/Y is the
// chrome-area offset (title bar + tabs + URL bar). Firefox's offsets differ
// from Chrome's — phase 04 re-measures them.
type Window struct {
	X, Y           int
	Scale          float64
	ContentOffsetX int
	ContentOffsetY int
}

// Config bundles knobs backends need at construction time.
type Config struct {
	Window Window
}

// Modifier is a bitmask of modifier keys for KeyCombo.
type Modifier uint32

const (
	ModCtrl  Modifier = 1 << iota // Control
	ModAlt                        // Alt / Option
	ModShift
	ModWin // Windows / Super / Meta
)

// Virtual-key codes for the most common non-character keys. Match Win32 VK_*
// values — the native backend forwards them as-is; the BiDi backend maps via
// vkToCode in keyboard.go.
const (
	VKReturn    uint16 = 0x0D
	VKBackspace uint16 = 0x08
	VKEscape    uint16 = 0x1B
	VKTab       uint16 = 0x09
	VKEnd       uint16 = 0x23
	VKHome      uint16 = 0x24
	VKLeft      uint16 = 0x25
	VKUp        uint16 = 0x26
	VKRight     uint16 = 0x27
	VKDown      uint16 = 0x28
	VKDelete    uint16 = 0x2E
	VKA         uint16 = 0x41
	VKL         uint16 = 0x4C
	VKT         uint16 = 0x54
)

// NewNative returns the platform-specific native backend (winapi on Windows,
// stub elsewhere). Exposed so the root firefoxkit package can construct one
// from per-Page configuration.
func NewNative(cfg Config) Backend { return newNative(cfg) }

// Backend is the low-level mouse/keyboard dispatcher.
type Backend interface {
	MouseMove(ctx context.Context, x, y float64) error
	MouseDown(ctx context.Context, x, y float64, button MouseButton) error
	MouseUp(ctx context.Context, x, y float64, button MouseButton) error
	MouseScroll(ctx context.Context, x, y, deltaX, deltaY float64) error
	KeyChar(ctx context.Context, ch rune) error
	InsertText(ctx context.Context, text string) error
	// KeyCombo presses a virtual-key code with optional modifiers held down,
	// then releases everything.
	KeyCombo(ctx context.Context, vk uint16, mods Modifier) error
}
