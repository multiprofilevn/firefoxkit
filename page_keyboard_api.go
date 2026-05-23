package firefoxkit

import (
	"context"

	"github.com/tuwibu/firefoxkit/input"
)

// Keyboard returns a per-page keyboard namespace. The wrapped *input.Keyboard
// is cached so timing knobs (MinDelay/MaxDelay) persist across calls.
func (p *Page) Keyboard() *Keyboard {
	p.kbOnce.Do(func() {
		p.kb = &Keyboard{p: p, k: input.NewKeyboard(p.inputBackend())}
	})
	return p.kb
}

// Keyboard groups keyboard shortcuts. Wraps input.Keyboard for cadence.
type Keyboard struct {
	p *Page
	k *input.Keyboard
}

// Type sends text with the default per-character delay.
func (k *Keyboard) Type(ctx context.Context, text string) error { return k.k.Type(ctx, text) }

// TypeFast bulk-inserts via one performActions call.
func (k *Keyboard) TypeFast(ctx context.Context, text string) error {
	return k.k.TypeFast(ctx, text)
}

// TypeHuman types with jittered cadence + occasional typos.
func (k *Keyboard) TypeHuman(ctx context.Context, text string, cfg ...input.HumanTypeConfig) error {
	return k.k.TypeHuman(ctx, text, cfg...)
}

// Press sends a single rune.
func (k *Keyboard) Press(ctx context.Context, ch rune) error { return k.k.Press(ctx, ch) }

// Combo presses VK with modifiers held.
func (k *Keyboard) Combo(ctx context.Context, vk uint16, mods input.Modifier) error {
	return k.k.Combo(ctx, vk, mods)
}

// ClearInput selects-all then deletes. Field must already have focus.
func (k *Keyboard) ClearInput(ctx context.Context) error { return k.k.ClearInput(ctx) }

// Down/Up are stubs until phase 04/05 surface a Backend split for held keys.
// For now they map onto KeyChar (one-shot down+up) — held-key semantics not
// required by any current public API path.
func (k *Keyboard) Down(ctx context.Context, ch rune) error {
	return k.k.Press(ctx, ch)
}

func (k *Keyboard) Up(_ context.Context, _ rune) error { return nil }
