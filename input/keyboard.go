package input

import (
	"context"
	"math/rand"
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

// vkToCode maps Win32 virtual-key codes to W3C WebDriver key values, so the
// BiDi backend can dispatch KeyCombo with the same VK_* constants the native
// backend uses.
var vkToCode = map[uint16]string{
	VKReturn:    KeyEnter,
	VKBackspace: KeyBackspace,
	VKEscape:    KeyEscape,
	VKTab:       KeyTab,
	VKEnd:       KeyEnd,
	VKHome:      KeyHome,
	VKLeft:      KeyArrowLeft,
	VKUp:        KeyArrowUp,
	VKRight:     KeyArrowRight,
	VKDown:      KeyArrowDown,
	VKDelete:    KeyDelete,
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
	for _, ch := range text {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := k.backend.KeyChar(ctx, ch); err != nil {
			return err
		}
		k.sleep()
	}
	return nil
}

// TypeFast asks the backend to bulk-insert.
func (k *Keyboard) TypeFast(ctx context.Context, text string) error {
	return k.backend.InsertText(ctx, text)
}

// Press sends a single rune.
func (k *Keyboard) Press(ctx context.Context, ch rune) error {
	return k.backend.KeyChar(ctx, ch)
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
