package bidi

import "context"

// InputSourceType is "key" | "pointer" | "wheel" | "none".
type InputSourceType string

const (
	SourceKey     InputSourceType = "key"
	SourcePointer InputSourceType = "pointer"
	SourceWheel   InputSourceType = "wheel"
	SourceNone    InputSourceType = "none"
)

// PointerType is "mouse" | "pen" | "touch". Mouse is the default.
type PointerType string

const (
	PointerMouse PointerType = "mouse"
	PointerPen   PointerType = "pen"
	PointerTouch PointerType = "touch"
)

// InputSource is one entry of the actions array. Type is "key" / "pointer" /
// "wheel" / "none"; ID is a free-form caller-supplied tag; Parameters carries
// pointer-only fields (pointerType). Actions is the ordered list of atomic
// events.
type InputSource struct {
	Type       InputSourceType  `json:"type"`
	ID         string           `json:"id"`
	Parameters *PointerParams   `json:"parameters,omitempty"`
	Actions    []map[string]any `json:"actions"`
}

// PointerParams is the optional parameters field for a pointer source.
type PointerParams struct {
	PointerType PointerType `json:"pointerType,omitempty"`
}

// PerformActionsParams configures input.performActions.
type PerformActionsParams struct {
	Context string        `json:"context"`
	Actions []InputSource `json:"actions"`
}

// PerformActions issues input.performActions for the given browsing context.
func (c *Client) PerformActions(ctx context.Context, p PerformActionsParams) error {
	return c.Send(ctx, "input.performActions", p, nil)
}

// ReleaseActionsParams configures input.releaseActions.
type ReleaseActionsParams struct {
	Context string `json:"context"`
}

// ReleaseActions resets pressed buttons / held keys for the context.
func (c *Client) ReleaseActions(ctx context.Context, p ReleaseActionsParams) error {
	return c.Send(ctx, "input.releaseActions", p, nil)
}

// --- Action builders (caller convenience) ----------------------------------

// PointerMove builds a pointerMove action map. origin defaults to "viewport".
func PointerMove(x, y float64, durationMs int) map[string]any {
	a := map[string]any{
		"type":   "pointerMove",
		"x":      x,
		"y":      y,
		"origin": "viewport",
	}
	if durationMs > 0 {
		a["duration"] = durationMs
	}
	return a
}

// PointerDown builds a pointerDown action map. button 0=left, 1=middle, 2=right.
func PointerDown(button int) map[string]any {
	return map[string]any{"type": "pointerDown", "button": button}
}

// PointerUp builds a pointerUp action map.
func PointerUp(button int) map[string]any {
	return map[string]any{"type": "pointerUp", "button": button}
}

// KeyDown builds a keyDown action map. value must be a single character or a
// W3C key code string (e.g. "" for Enter — see WebDriver key code page).
func KeyDown(value string) map[string]any {
	return map[string]any{"type": "keyDown", "value": value}
}

// KeyUp builds a keyUp action map.
func KeyUp(value string) map[string]any {
	return map[string]any{"type": "keyUp", "value": value}
}

// Pause builds a pause (idle) action map. Used to insert delays between
// events without changing input source state.
func Pause(durationMs int) map[string]any {
	return map[string]any{"type": "pause", "duration": durationMs}
}

// Scroll builds a wheel scroll action map. x/y is the origin (viewport
// coords); deltaX/deltaY is the scroll distance in CSS pixels.
func Scroll(x, y, deltaX, deltaY float64, durationMs int) map[string]any {
	a := map[string]any{
		"type":   "scroll",
		"x":      x,
		"y":      y,
		"deltaX": deltaX,
		"deltaY": deltaY,
		"origin": "viewport",
	}
	if durationMs > 0 {
		a["duration"] = durationMs
	}
	return a
}
