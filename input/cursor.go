package input

// Profile selects how Cursor.MoveTo paints the path between two points.
// Only meaningful for the native backend; the BiDi backend dispatches via
// input.performActions in a single shot and ignores the profile.
type Profile int

const (
	// ProfileHuman draws a Bezier curve with ease-in-out (native backend only).
	ProfileHuman Profile = iota
	// ProfileFast uses linear interpolation with minimal sleep.
	ProfileFast
	// ProfileMachine moves in one shot — no interpolation.
	ProfileMachine
)
