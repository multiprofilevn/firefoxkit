package input

// CursorTuning caps ProfileHuman's step count and per-point delay range.
// Zero-value reproduces the current hardcoded behavior (MaxSteps 80,
// delay 8-18ms) — setting it has no effect unless a field is non-zero.
// Affects ProfileHuman only; ProfileFast and ProfileMachine are untouched.
type CursorTuning struct {
	// MaxSteps caps the bezier path's point count. <= 0 keeps the default (80).
	MaxSteps int
	// MinDelayMs, MaxDelayMs bound the per-point delay in milliseconds. Both
	// <= 0 keeps the default range (8-18ms). MaxDelayMs < MinDelayMs collapses
	// to a fixed MinDelayMs delay.
	MinDelayMs, MaxDelayMs int
}

// SetTuning overrides the ProfileHuman step/delay caps for subsequent moves.
// Safe to call concurrently with MoveTo.
func (c *Cursor) SetTuning(t CursorTuning) {
	c.mu.Lock()
	c.tuning = t
	c.mu.Unlock()
}
