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

// ErrForegroundLost signals the target window no longer owns the OS foreground,
// so the next keystroke would land in whatever window took it. Emission stops
// at once — SendInput is not addressed to a window, so continuing would type
// into a foreign application.
var ErrForegroundLost = errors.New("input: foreground lost")

// GuardChunkSize is how many characters may be emitted between two
// ForegroundGuard checks. It bounds the leak: losing the foreground mid-string
// can spill at most this many keystrokes into a foreign window, in exchange for
// keeping the check off the per-keystroke path.
const GuardChunkSize = 5

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

// Rect is a window rectangle in absolute screen pixels, as reported by Win32
// GetWindowRect. It is an OUT-OF-BAND measurement: it comes from the window
// manager, not from anything the page can script, which is what makes it usable
// as a sanity bound on offsets measured inside the page realm.
type Rect struct {
	Left, Top, Right, Bottom int
}

// Config bundles knobs backends need at construction time.
type Config struct {
	Window Window
	// ForegroundGuard is consulted by the lowest-level emitters (sendKey,
	// sendVK, sendMouse) before input reaches the OS. A non-nil return aborts
	// the emission and propagates to the caller; return ErrForegroundLost when
	// the target window no longer owns the foreground. nil disables the check.
	//
	// Placing it at the emitters rather than at Keyboard means ClearInput,
	// Combo, Press, Type and TypeHuman are covered structurally — no call site
	// has to remember. Keyboard emissions are checked once per GuardChunkSize
	// characters; mouse button events are checked every time (they are rare).
	ForegroundGuard func() error
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
	VKPageUp    uint16 = 0x21
	VKPageDown  uint16 = 0x22
	VKEnd       uint16 = 0x23
	VKHome      uint16 = 0x24
	VKLeft      uint16 = 0x25
	VKUp        uint16 = 0x26
	VKRight     uint16 = 0x27
	VKDown      uint16 = 0x28
	VKDelete    uint16 = 0x2E
	VKSpace     uint16 = 0x20
	// Modifier virtual-key codes.
	VKShift   uint16 = 0x10
	VKControl uint16 = 0x11
	VKMenu    uint16 = 0x12 // Alt
	VKWin     uint16 = 0x5B // Left Windows key
	// Letter keys used by ClearInput and combos.
	VKA uint16 = 0x41
	VKL uint16 = 0x4C
	VKT uint16 = 0x54
	// Function keys F1–F12.
	VKF1  uint16 = 0x70
	VKF2  uint16 = 0x71
	VKF3  uint16 = 0x72
	VKF4  uint16 = 0x73
	VKF5  uint16 = 0x74
	VKF6  uint16 = 0x75
	VKF7  uint16 = 0x76
	VKF8  uint16 = 0x77
	VKF9  uint16 = 0x78
	VKF10 uint16 = 0x79
	VKF11 uint16 = 0x7A
	VKF12 uint16 = 0x7B
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
	// SetWindow replaces the coordinate frame this backend maps CSS pixels
	// through, IN PLACE. It exists so a re-measured frame can reach a backend
	// that is already wired into a Cursor / Keyboard / TouchScreen without
	// replacing the backend object: the page caches the backend behind three
	// separate sync.Once values (bkCache, cursorOnce, kbOnce), so rebuilding
	// one of them leaves the other two pointing at the stale frame — and
	// swapping a backend between a MouseDown and its MouseUp leaves the button
	// held down at the OS level forever.
	//
	// Implementations MUST be safe to call while another goroutine is mid-op
	// and MUST NOT block that op for meaningfully long. Backends that dispatch
	// over the wire (BiDi) have no screen frame and no-op.
	SetWindow(w Window)
}
