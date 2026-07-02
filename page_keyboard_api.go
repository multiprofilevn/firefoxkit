package firefoxkit

import (
	"github.com/tuwibu/firefoxkit/input"
)

// Keyboard returns the per-page KeyboardAPI, cached so timing knobs persist.
func (p *Page) Keyboard() *KeyboardAPI {
	p.kbOnce.Do(func() {
		p.kb = &KeyboardAPI{p: p, k: input.NewKeyboard(p.inputBackend())}
	})
	return p.kb
}

// KeyboardAPI groups keyboard primitives. The embedded *Page context is used
// internally via p.ctx() — callers never pass context.Context.
type KeyboardAPI struct {
	p *Page
	k *input.Keyboard
}

// Press dispatches keydown + keyup for the named key, holding mods for the
// duration. key accepts a named key ("Enter", "Tab", "ArrowDown", "F1", …) or
// a single printable character ("a", "1", "?"). Modifier flags are OR-folded.
func (k *KeyboardAPI) Press(key string, mods ...input.Modifier) error {
	return k.pressKey(key, mods, true, true)
}

// Down dispatches keydown without releasing. Pair with Up to simulate held keys.
func (k *KeyboardAPI) Down(key string, mods ...input.Modifier) error {
	return k.pressKey(key, mods, true, false)
}

// Up dispatches keyup. Caller is responsible for balancing prior Down calls.
func (k *KeyboardAPI) Up(key string, mods ...input.Modifier) error {
	return k.pressKey(key, mods, false, true)
}

// pressKey is the shared dispatch for Press/Down/Up. sendDown / sendUp control
// which half of the keyDown+keyUp pair to emit.
//
// Strategy: named keys → KeyCombo (vk + mods via existing BiDi path).
// Printable single-char keys → KeyChar for down+up; for bare Down/Up we use
// the BiDi performActions path directly via the keyString helper so the split
// is visible to the browser.
func (k *KeyboardAPI) pressKey(key string, mods []input.Modifier, sendDown, sendUp bool) error {
	ctx := k.p.ctx()

	entry, ok := resolveKey(key)
	if !ok {
		return input.ErrInvalidKey
	}

	// Combine modifier bitmask.
	var mod input.Modifier
	for _, m := range mods {
		mod |= m
	}

	// Printable single-char path with no modifiers: use KeyChar (down+up).
	// For Down-only or Up-only we fall through to the combo path using the
	// character's w3cKey value.
	if entry.isPrintable && mod == 0 && sendDown && sendUp {
		runes := []rune(entry.w3cKey)
		if len(runes) == 1 {
			return k.k.Press(ctx, runes[0])
		}
	}

	// Named-key path (and printable-with-mods, and Down/Up splits):
	// Use KeyCombo when vk is known; fall back to the w3cKey string when
	// only the BiDi backend is available (no native VK mapping needed).
	if entry.vk != 0 {
		// Down+Up in one shot → standard KeyCombo.
		if sendDown && sendUp {
			return k.k.Combo(ctx, entry.vk, mod)
		}
		// Split Down/Up: emit via backend directly so the keyDown or keyUp
		// is sent without its counterpart. The BiDi backend sends both in
		// KeyCombo, so for split semantics we use KeyComboSplit if available,
		// or fall back to KeyCombo (best-effort, see DONE_WITH_CONCERNS).
		if bidi, ok := k.p.inputBackend().(*input.BackendBiDi); ok {
			modKeys := input.ModifierKeys(mod)
			if sendDown {
				return bidi.KeyDown(ctx, entry.vk, modKeys)
			}
			return bidi.KeyUp(ctx, entry.vk, modKeys)
		}
		// Native backend or unknown: full combo is best-effort for split.
		return k.k.Combo(ctx, entry.vk, mod)
	}

	// Printable char with mods, no vk mapping.
	// For the BiDi backend we can wrap modifier keys around the char action so
	// the modifier is actually applied (e.g. Press("a", ModShift) sends Shift+a).
	runes := []rune(entry.w3cKey)
	if len(runes) == 1 {
		if sendDown && sendUp {
			if bidiB, ok := k.p.inputBackend().(*input.BackendBiDi); ok && mod != 0 {
				// modDown(s) → keyDown(char) → keyUp(char) → modUp(s)
				return bidiB.KeyCharWithMods(ctx, runes[0], input.ModifierKeys(mod))
			}
			return k.k.Press(ctx, runes[0])
		}
	}
	return nil
}

// Type sends text character-by-character with a random per-key delay.
func (k *KeyboardAPI) Type(text string) error {
	return k.k.Type(k.p.ctx(), text)
}

// TypeFast bulk-inserts text in one performActions call.
func (k *KeyboardAPI) TypeFast(text string) error {
	return k.k.TypeFast(k.p.ctx(), text)
}

// TypeHuman types with jittered cadence and occasional simulated typos.
func (k *KeyboardAPI) TypeHuman(text string, cfg ...input.HumanTypeConfig) error {
	return k.k.TypeHuman(k.p.ctx(), text, cfg...)
}

// TypeSelector focuses sel, clears it, then types text via TypeHuman with a
// verify-and-retry loop. Returns ErrTypeSelectorMismatch on persistent failure.
func (k *KeyboardAPI) TypeSelector(sel, text string, cfg ...TypeSelectorConfig) error {
	return typeSelectorImpl(k, sel, text, cfg...)
}

// ClearInput wipes the focused input via Ctrl+A then Delete. When selector is
// provided the element is focused first; otherwise the current active element
// is targeted (caller must ensure something useful has focus).
func (k *KeyboardAPI) ClearInput(selector ...string) error {
	if len(selector) > 0 && selector[0] != "" {
		if err := k.p.Focus(selector[0]); err != nil {
			return err
		}
	}
	return k.k.ClearInput(k.p.ctx())
}
