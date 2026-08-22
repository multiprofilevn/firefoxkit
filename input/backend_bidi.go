package input

import (
	"context"
	"fmt"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// BackendBiDi dispatches input via BiDi input.performActions.
// Constructed per Page — each instance is bound to one browsingContext id.
type BackendBiDi struct {
	client     *bidi.Client
	contextID  string
	pointerID  string
	keyboardID string
	wheelID    string
}

// SetWindow is a no-op. BiDi dispatches into the browsing context by CSS
// coordinate; it never touches the OS pointer, so it has no screen coordinate
// frame to update.
func (b *BackendBiDi) SetWindow(Window) {}

// NewBiDi returns a BackendBiDi bound to the given browsing context.
func NewBiDi(client *bidi.Client, contextID string) *BackendBiDi {
	return &BackendBiDi{
		client:     client,
		contextID:  contextID,
		pointerID:  "ffk-mouse",
		keyboardID: "ffk-keyboard",
		wheelID:    "ffk-wheel",
	}
}

func (b *BackendBiDi) pointerSource(actions ...map[string]any) bidi.InputSource {
	return bidi.InputSource{
		Type:       bidi.SourcePointer,
		ID:         b.pointerID,
		Parameters: &bidi.PointerParams{PointerType: bidi.PointerMouse},
		Actions:    actions,
	}
}

func (b *BackendBiDi) keySource(actions ...map[string]any) bidi.InputSource {
	return bidi.InputSource{
		Type:    bidi.SourceKey,
		ID:      b.keyboardID,
		Actions: actions,
	}
}

func (b *BackendBiDi) wheelSource(actions ...map[string]any) bidi.InputSource {
	return bidi.InputSource{
		Type:    bidi.SourceWheel,
		ID:      b.wheelID,
		Actions: actions,
	}
}

func (b *BackendBiDi) perform(ctx context.Context, sources ...bidi.InputSource) error {
	return b.client.PerformActions(ctx, bidi.PerformActionsParams{
		Context: b.contextID,
		Actions: sources,
	})
}

// MouseMove moves the cursor to (x,y) in viewport CSS pixels.
func (b *BackendBiDi) MouseMove(ctx context.Context, x, y float64) error {
	return b.perform(ctx, b.pointerSource(bidi.PointerMove(x, y, 0)))
}

// MouseDown presses the button at (x,y).
func (b *BackendBiDi) MouseDown(ctx context.Context, x, y float64, button MouseButton) error {
	return b.perform(ctx, b.pointerSource(
		bidi.PointerMove(x, y, 0),
		bidi.PointerDown(int(button)),
	))
}

// MouseUp releases the button at (x,y).
func (b *BackendBiDi) MouseUp(ctx context.Context, x, y float64, button MouseButton) error {
	return b.perform(ctx, b.pointerSource(
		bidi.PointerMove(x, y, 0),
		bidi.PointerUp(int(button)),
	))
}

// MouseScroll dispatches a wheel scroll from (x,y) by (deltaX, deltaY).
func (b *BackendBiDi) MouseScroll(ctx context.Context, x, y, deltaX, deltaY float64) error {
	return b.perform(ctx, b.wheelSource(bidi.Scroll(x, y, deltaX, deltaY, 0)))
}

// KeyChar sends a single character (down + up).
func (b *BackendBiDi) KeyChar(ctx context.Context, ch rune) error {
	v := string(ch)
	return b.perform(ctx, b.keySource(
		bidi.KeyDown(v),
		bidi.KeyUp(v),
	))
}

// KeyCharWithMods sends modDown(s) → keyDown(ch) → keyUp(ch) → modUp(s) so
// that modifier keys are held for the character press. Used by KeyboardAPI
// when a printable char has no VK mapping but modifiers are requested.
func (b *BackendBiDi) KeyCharWithMods(ctx context.Context, ch rune, modKeys []string) error {
	v := string(ch)
	actions := make([]map[string]any, 0, len(modKeys)*2+2)
	for _, m := range modKeys {
		actions = append(actions, bidi.KeyDown(m))
	}
	actions = append(actions, bidi.KeyDown(v), bidi.KeyUp(v))
	for i := len(modKeys) - 1; i >= 0; i-- {
		actions = append(actions, bidi.KeyUp(modKeys[i]))
	}
	return b.perform(ctx, b.keySource(actions...))
}

// InsertText sends a sequence of characters as one performActions call —
// faster than per-char dispatch.
func (b *BackendBiDi) InsertText(ctx context.Context, text string) error {
	actions := make([]map[string]any, 0, len(text)*2)
	for _, ch := range text {
		v := string(ch)
		actions = append(actions, bidi.KeyDown(v), bidi.KeyUp(v))
	}
	return b.perform(ctx, b.keySource(actions...))
}

// KeyCombo presses VK with modifiers held: modDown(s) → keyDown(VK) → keyUp(VK) → modUp(s).
func (b *BackendBiDi) KeyCombo(ctx context.Context, vk uint16, mods Modifier) error {
	target, ok := VKValue(vk)
	if !ok {
		return fmt.Errorf("%w: vk=0x%X", ErrInvalidKey, vk)
	}
	modKeys := ModifierKeys(mods)
	actions := make([]map[string]any, 0, len(modKeys)*2+2)
	for _, m := range modKeys {
		actions = append(actions, bidi.KeyDown(m))
	}
	actions = append(actions, bidi.KeyDown(target), bidi.KeyUp(target))
	// Release modifiers in reverse order to mirror real keyboard behavior.
	for i := len(modKeys) - 1; i >= 0; i-- {
		actions = append(actions, bidi.KeyUp(modKeys[i]))
	}
	return b.perform(ctx, b.keySource(actions...))
}

// KeyDown sends modDown(s) → keyDown(target) without releasing. Pair with
// KeyUp to simulate held keys. modKeys must match what was passed to KeyDown.
func (b *BackendBiDi) KeyDown(ctx context.Context, vk uint16, modKeys []string) error {
	target, ok := VKValue(vk)
	if !ok {
		return fmt.Errorf("%w: vk=0x%X", ErrInvalidKey, vk)
	}
	actions := make([]map[string]any, 0, len(modKeys)+1)
	for _, m := range modKeys {
		actions = append(actions, bidi.KeyDown(m))
	}
	actions = append(actions, bidi.KeyDown(target))
	return b.perform(ctx, b.keySource(actions...))
}

// KeyUp sends keyUp(target) → modUp(s) in reverse order. modKeys must match
// what was passed to the corresponding KeyDown call.
func (b *BackendBiDi) KeyUp(ctx context.Context, vk uint16, modKeys []string) error {
	target, ok := VKValue(vk)
	if !ok {
		return fmt.Errorf("%w: vk=0x%X", ErrInvalidKey, vk)
	}
	actions := make([]map[string]any, 0, len(modKeys)+1)
	actions = append(actions, bidi.KeyUp(target))
	for i := len(modKeys) - 1; i >= 0; i-- {
		actions = append(actions, bidi.KeyUp(modKeys[i]))
	}
	return b.perform(ctx, b.keySource(actions...))
}

// Release issues input.releaseActions — call this on Page close to clear
// any held keys / pressed buttons.
func (b *BackendBiDi) Release(ctx context.Context) error {
	return b.client.ReleaseActions(ctx, bidi.ReleaseActionsParams{Context: b.contextID})
}

// Compile-time interface assertion.
var _ Backend = (*BackendBiDi)(nil)
