package input

// CursorProfile selects the cursor movement style. Only meaningful for the
// native backend; the BiDi backend dispatches via input.performActions in a
// single shot and ignores the profile.
type CursorProfile int

const (
	// CursorHuman draws a Bezier path with ease-in-out (native backend only).
	CursorHuman CursorProfile = iota
	// CursorFast uses a short linear path.
	CursorFast
	// CursorMachine moves in one shot (no intermediate steps).
	CursorMachine
)
