package input

import (
	"context"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// PathPoint is one step of a cursor path: target coordinate + how long to
// pause after reaching it.
type PathPoint struct {
	X, Y    float64
	DelayMs int
}

// PathBatcher is implemented by backends that can replay a whole cursor path
// in a single (or few) protocol calls instead of one RPC per point. Batching
// removes the per-point round-trip flood that congests the shared WebSocket
// when many goroutines/pages move the mouse at once (the chromedp-style
// "mouse stops moving under load" failure).
type PathBatcher interface {
	MousePath(ctx context.Context, pts []PathPoint) error
}

// mousePathChunkActions caps actions per input.performActions call. Each point
// emits up to 2 actions (move + pause), so one call carries ≤ ~100 points.
const mousePathChunkActions = 200

// MousePath replays pts as pointerMove+pause action sequences, chunked into
// few performActions calls. Firefox handles the timing — no Go-side sleeps,
// no per-point round-trips.
func (b *BackendBiDi) MousePath(ctx context.Context, pts []PathPoint) error {
	actions := make([]map[string]any, 0, mousePathChunkActions)
	flush := func() error {
		if len(actions) == 0 {
			return nil
		}
		err := b.perform(ctx, b.pointerSource(actions...))
		actions = actions[:0]
		return err
	}
	for _, p := range pts {
		actions = append(actions, bidi.PointerMove(p.X, p.Y, 0))
		if p.DelayMs > 0 {
			actions = append(actions, bidi.Pause(p.DelayMs))
		}
		if len(actions) >= mousePathChunkActions {
			if err := flush(); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}
	return flush()
}
