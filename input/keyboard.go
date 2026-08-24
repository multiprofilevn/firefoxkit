package input

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// W3C WebDriver "Keyboard Keypress" Unicode codepoints. These map to the
// `value` field of a BiDi keyDown / keyUp action when sending non-printable
// keys. Printable characters are sent verbatim.
const (
	KeyNull       = ""
	KeyBackspace  = ""
	KeyTab        = ""
	KeyReturn     = ""
	KeyEnter      = ""
	KeyShift      = ""
	KeyControl    = ""
	KeyAlt        = ""
	KeyEscape     = ""
	KeySpace      = ""
	KeyPageUp     = ""
	KeyPageDown   = ""
	KeyEnd        = ""
	KeyHome       = ""
	KeyArrowLeft  = ""
	KeyArrowUp    = ""
	KeyArrowRight = ""
	KeyArrowDown  = ""
	KeyDelete     = ""
	KeyMeta       = ""
)

// W3C WebDriver function-key Unicode codepoints (U+E031–U+E03C).
const (
	KeyF1  = ""
	KeyF2  = ""
	KeyF3  = ""
	KeyF4  = ""
	KeyF5  = ""
	KeyF6  = ""
	KeyF7  = ""
	KeyF8  = ""
	KeyF9  = ""
	KeyF10 = ""
	KeyF11 = ""
	KeyF12 = ""
)

// vkToCode maps Win32 virtual-key codes to W3C WebDriver key values, so the
// BiDi backend can dispatch KeyCombo with the same VK_* constants the native
// backend uses.
var vkToCode = map[uint16]string{
	VKReturn:    KeyEnter,
	VKBackspace: KeyBackspace,
	VKEscape:    KeyEscape,
	VKTab:       KeyTab,
	VKPageUp:    KeyPageUp,
	VKPageDown:  KeyPageDown,
	VKEnd:       KeyEnd,
	VKHome:      KeyHome,
	VKLeft:      KeyArrowLeft,
	VKUp:        KeyArrowUp,
	VKRight:     KeyArrowRight,
	VKDown:      KeyArrowDown,
	VKDelete:    KeyDelete,
	VKSpace:     KeySpace,
	VKShift:     KeyShift,
	VKControl:   KeyControl,
	VKMenu:      KeyAlt,
	VKWin:       KeyMeta,
	VKF1:        KeyF1,
	VKF2:        KeyF2,
	VKF3:        KeyF3,
	VKF4:        KeyF4,
	VKF5:        KeyF5,
	VKF6:        KeyF6,
	VKF7:        KeyF7,
	VKF8:        KeyF8,
	VKF9:        KeyF9,
	VKF10:       KeyF10,
	VKF11:       KeyF11,
	VKF12:       KeyF12,
}

// vkToRune maps the printable-letter VKs to their lowercase Unicode rune.
var vkToRune = map[uint16]rune{
	VKA: 'a',
	VKL: 'l',
	VKT: 't',
}

// VKValue resolves a VK_* code to the BiDi key `value` string.
func VKValue(vk uint16) (string, bool) {
	if s, ok := vkToCode[vk]; ok {
		return s, true
	}
	if r, ok := vkToRune[vk]; ok {
		return string(r), true
	}
	return "", false
}

// ModifierKeys returns the W3C key values for each bit set in mods. Order:
// Ctrl, Shift, Alt, Win.
func ModifierKeys(mods Modifier) []string {
	keys := make([]string, 0, 4)
	if mods&ModCtrl != 0 {
		keys = append(keys, KeyControl)
	}
	if mods&ModShift != 0 {
		keys = append(keys, KeyShift)
	}
	if mods&ModAlt != 0 {
		keys = append(keys, KeyAlt)
	}
	if mods&ModWin != 0 {
		keys = append(keys, KeyMeta)
	}
	return keys
}

// Keyboard adds human-like typing on top of Backend's per-rune key dispatch.
type Keyboard struct {
	backend            Backend
	MinDelay, MaxDelay time.Duration
}

// NewKeyboard returns a Keyboard with default 50-150ms range.
func NewKeyboard(b Backend) *Keyboard {
	return &Keyboard{
		backend:  b,
		MinDelay: 50 * time.Millisecond,
		MaxDelay: 150 * time.Millisecond,
	}
}

// Type sends each rune via the backend with a random per-key delay.
func (k *Keyboard) Type(ctx context.Context, text string) error {
	for _, ch := range normalizeLineBreaks(text) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := k.emitRune(ctx, ch); err != nil {
			return err
		}
		k.sleep()
	}
	return nil
}

// normalizeLineBreaks folds CRLF and a lone CR into LF, so one line break is
// one rune — and therefore one Enter press — whatever convention the caller's
// text arrived in. Text submitted from an HTML textarea carries CRLF.
func normalizeLineBreaks(text string) string {
	if !strings.ContainsRune(text, '\r') {
		return text
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// emitRune dispatches one rune of typed text. A line break is not a printable
// character, and routing it through KeyChar gets it wrong on both backends.
//
// The remote backend sends a line break as a printable key value, so an editor
// that consumes Enter in its own keydown handler (Draft.js, and every rich
// composer built on it) acts on the key AND still receives the character,
// turning one break in the source text into two on the page. The native
// backend injects U+000A as a Unicode scan code, which the browser drops on
// the floor, losing the break entirely.
//
// Both backends agree on VK_RETURN, so a line break goes out as the key a
// human would actually press.
func (k *Keyboard) emitRune(ctx context.Context, ch rune) error {
	if ch == '\n' || ch == '\r' {
		return k.backend.KeyCombo(ctx, VKReturn, 0)
	}
	return k.backend.KeyChar(ctx, ch)
}

// TypeFast asks the backend to bulk-insert.
func (k *Keyboard) TypeFast(ctx context.Context, text string) error {
	return k.backend.InsertText(ctx, text)
}

// Press sends a single rune. A line break goes out as an Enter press — see
// emitRune.
func (k *Keyboard) Press(ctx context.Context, ch rune) error {
	return k.emitRune(ctx, ch)
}

// Combo presses VK with modifiers held.
func (k *Keyboard) Combo(ctx context.Context, vk uint16, mods Modifier) error {
	return k.backend.KeyCombo(ctx, vk, mods)
}

// ClearInput selects all then deletes.
func (k *Keyboard) ClearInput(ctx context.Context) error {
	if err := k.backend.KeyCombo(ctx, VKA, ModCtrl); err != nil {
		return err
	}
	return k.backend.KeyCombo(ctx, VKDelete, 0)
}

func (k *Keyboard) sleep() {
	mn := int(k.MinDelay / time.Millisecond)
	mx := int(k.MaxDelay / time.Millisecond)
	if mx <= mn {
		time.Sleep(k.MinDelay)
		return
	}
	time.Sleep(time.Duration(mn+rand.Intn(mx-mn)) * time.Millisecond)
}
