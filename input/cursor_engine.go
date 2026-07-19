package input

import (
	"context"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// remotePathTimeout bounds a single mouse-positions call so a slow/dead API
// fails fast to the local bezier fallback instead of stalling every move.
const remotePathTimeout = 1200 * time.Millisecond

// maxRemoteDelayMs clamps the per-point delay a mouse-positions server may
// request. A buggy/adversarial server returning huge delays would otherwise
// wedge every move for minutes.
const maxRemoteDelayMs = 50

// maxRemotePathPoints rejects absurdly long server paths (fall back to the
// local bezier) — they would turn one MoveTo into thousands of actions.
const maxRemotePathPoints = 2000

// Cursor adds a human-like layer over Backend's mouse primitives.
// Tracks position so MoveTo can interpolate from where it left off.
// All gesture methods serialize on an internal mutex: two goroutines moving
// the same cursor take turns instead of interleaving their paths.
type Cursor struct {
	backend Backend
	profile Profile

	mu   sync.Mutex
	x, y float64

	// mousePathURL, when non-empty, is the full URL of a mouse-positions
	// endpoint that returns a high-entropy {x,y,delay} path. ProfileHuman
	// fetches from it and replays; any failure falls back to the local bezier.
	mousePathURL string
	httpc        *http.Client
}

// NewCursor builds a Cursor with the default human profile.
func NewCursor(b Backend) *Cursor { return &Cursor{backend: b, profile: ProfileHuman} }

// NewCursorWithProfile builds a Cursor with an explicit movement profile.
func NewCursorWithProfile(b Backend, p Profile) *Cursor {
	return &Cursor{backend: b, profile: p}
}

// NewCursorWithProfileAndPath builds a Cursor that, on ProfileHuman, routes
// moves through a remote mouse-positions endpoint (mousePathURL). Empty URL
// behaves exactly like NewCursorWithProfile (pure local bezier).
func NewCursorWithProfileAndPath(b Backend, p Profile, mousePathURL string) *Cursor {
	c := &Cursor{backend: b, profile: p, mousePathURL: mousePathURL}
	if mousePathURL != "" {
		c.httpc = &http.Client{Timeout: remotePathTimeout}
	}
	return c
}

// Position returns the last x, y the cursor was moved to.
func (c *Cursor) Position() (float64, float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.x, c.y
}

// SetPosition updates the tracked position without emitting movement. Use
// after a raw-backend gesture (e.g. a drag's press-hold-release that bypasses
// MoveTo) so the next MoveTo seeds its bezier from where the gesture actually
// ended, not where the cursor was before it. Backends with OSPositionCSS
// re-read the real position anyway; this keeps the BiDi backend (no OS cursor)
// in sync.
func (c *Cursor) SetPosition(x, y float64) {
	c.mu.Lock()
	c.x, c.y = x, y
	c.mu.Unlock()
}

// cursorSeeder is implemented by backends that have a real OS cursor — they
// can report its current position in the backend's CSS coordinate space.
// Lets MoveTo start the path from where the cursor physically is, not from
// the last CSS coord this Cursor instance happened to set. Critical when
// multiple Cursor instances share one OS pointer (e.g. one Cursor per browser
// in chromefleet) — without this they all teleport between browsers.
type cursorSeeder interface {
	OSPositionCSS() (float64, float64, bool)
}

// MoveTo glides from the current position to (x, y). Behavior depends on the
// configured profile (ProfileHuman by default).
func (c *Cursor) MoveTo(ctx context.Context, x, y float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.moveToLocked(ctx, x, y)
}

// moveToLocked is MoveTo's body; caller must hold c.mu.
func (c *Cursor) moveToLocked(ctx context.Context, x, y float64) error {
	if seeder, ok := c.backend.(cursorSeeder); ok {
		if sx, sy, ok := seeder.OSPositionCSS(); ok {
			c.x, c.y = sx, sy
		}
	}
	defer func() { c.x, c.y = x, y }()

	if c.profile == ProfileMachine {
		if err := ctx.Err(); err != nil {
			return err
		}
		return c.backend.MouseMove(ctx, x, y)
	}

	pts := c.buildPath(ctx, x, y)

	// Batching backend (BiDi): replay the whole path in one/few protocol
	// calls — no per-point round-trips, no Go-side sleeps. Under fleet load
	// this is what keeps the shared WebSocket from flooding with tiny RPCs.
	if pb, ok := c.backend.(PathBatcher); ok {
		return pb.MousePath(ctx, pts)
	}

	// Per-point fallback (native backend: a real OS cursor can't be batched).
	for _, p := range pts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.backend.MouseMove(ctx, p.X, p.Y); err != nil {
			return err
		}
		if err := sleepCtx(ctx, time.Duration(p.DelayMs)*time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}

// buildPath produces the point sequence for the active profile, starting from
// the tracked position. Caller must hold c.mu.
func (c *Cursor) buildPath(ctx context.Context, x, y float64) []PathPoint {
	switch c.profile {
	case ProfileFast:
		steps := stepCount(c.x, c.y, x, y)
		return withDelays(interpolate(c.x, c.y, x, y, steps), func() int { return 2 + rand.Intn(4) })

	default: // ProfileHuman
		if c.mousePathURL != "" {
			if pts, err := fetchRemotePath(ctx, c.httpc, c.mousePathURL, c.x, c.y, x, y); err == nil && validRemotePath(pts, x, y) {
				out := make([]PathPoint, len(pts))
				for i, p := range pts {
					d := int(p.Delay)
					if d > maxRemoteDelayMs {
						d = maxRemoteDelayMs
					}
					out[i] = PathPoint{X: p.X, Y: p.Y, DelayMs: d}
				}
				return out
			}
		}
		steps := humanStepCount(c.x, c.y, x, y)
		return withDelays(bezierPath(c.x, c.y, x, y, steps), func() int { return 8 + rand.Intn(11) })
	}
}

// withDelays attaches a per-point delay drawn from next() to each path point.
func withDelays(pts []Point, next func() int) []PathPoint {
	out := make([]PathPoint, len(pts))
	for i, p := range pts {
		out[i] = PathPoint{X: p.X, Y: p.Y, DelayMs: next()}
	}
	return out
}

// Click moves to (x, y) and performs a press/release with a random hold time.
func (c *Cursor) Click(ctx context.Context, x, y float64, b MouseButton) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.moveToLocked(ctx, x, y); err != nil {
		return err
	}
	if err := c.backend.MouseDown(ctx, x, y, b); err != nil {
		return err
	}
	if err := sleepCtx(ctx, time.Duration(80+rand.Intn(50))*time.Millisecond); err != nil {
		// Never leave the button pressed on cancellation — the caller ctx is
		// already dead, so release on a short background ctx.
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.backend.MouseUp(rctx, x, y, b)
		cancel()
		return err
	}
	return c.backend.MouseUp(ctx, x, y, b)
}

// Scroll dispatches a wheel event from the cursor's current position.
func (c *Cursor) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	c.mu.Lock()
	x, y := c.x, c.y
	c.mu.Unlock()
	return c.backend.MouseScroll(ctx, x, y, deltaX, deltaY)
}
