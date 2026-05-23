package firefoxkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/tuwibu/firefoxkit/input"
)

// TypeSelectorConfig controls TypeSelector's verify-and-retry loop.
type TypeSelectorConfig struct {
	// Strict requires the final input value to equal text exactly. When false
	// a substring match is accepted (lets autocomplete widgets pass).
	Strict bool
	// MaxAttempts caps how many times the loop will Clear→Type→Verify before
	// giving up with ErrTypeSelectorMismatch. Default 3.
	MaxAttempts int
	// HumanCfg tunes the per-char cadence. Zero values use TypeHuman defaults.
	HumanCfg input.HumanTypeConfig
}

func (c TypeSelectorConfig) withDefaults() TypeSelectorConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	return c
}

// TypeSelector focuses sel, clears, types text via TypeHuman, then reads the
// element's value back and retries on mismatch. Returns nil on success,
// ErrTypeSelectorMismatch on persistent mismatch, ErrPageNotFound if sel misses.
func (k *Keyboard) TypeSelector(ctx context.Context, sel, text string, cfg ...TypeSelectorConfig) error {
	c := TypeSelectorConfig{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	c = c.withDefaults()

	for attempt := 0; attempt < c.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := k.p.Focus(ctx, sel); err != nil {
			return err
		}
		if err := k.ClearInput(ctx); err != nil {
			return err
		}
		if err := k.TypeHuman(ctx, text, c.HumanCfg); err != nil {
			return err
		}
		var got string
		if err := k.p.EvalSelector(ctx, sel, `(el) => el.value ?? el.textContent ?? ""`, &got); err != nil {
			return err
		}
		if c.Strict {
			if got == text {
				return nil
			}
		} else if got == text || strings.Contains(got, text) {
			return nil
		}
	}
	return fmt.Errorf("%w: sel=%q after %d attempts", ErrTypeSelectorMismatch, sel, c.MaxAttempts)
}
