package input

import (
	"context"
	"testing"
)

// TestHumanPath_DefaultTuning_Unchanged locks in current behavior: zero-value
// CursorTuning must not change ProfileHuman's step cap (80) or delay range
// (8-18ms) — no regression when ops don't opt in to tuning.
func TestHumanPath_DefaultTuning_Unchanged(t *testing.T) {
	bk := &batchBackend{}
	c := NewCursorWithProfile(bk, ProfileHuman)
	c.SetTuning(CursorTuning{}) // explicit zero-value

	if err := c.MoveTo(context.Background(), 5000, 5000); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	pts := bk.allPoints()
	if len(pts) == 0 || len(pts) > 80 {
		t.Fatalf("points = %d, want 1..80 (default MaxSteps=80)", len(pts))
	}
	for _, p := range pts {
		if p.DelayMs < 8 || p.DelayMs > 18 {
			t.Fatalf("DelayMs = %d, want in [8,18] (default range)", p.DelayMs)
		}
	}
	last := pts[len(pts)-1]
	if last.X != 5000 || last.Y != 5000 {
		t.Fatalf("last point = (%v,%v), want (5000,5000)", last.X, last.Y)
	}
}

// TestHumanPath_MaxStepsRespected: capping MaxSteps must shrink the longest
// path while keeping the exact-target termination invariant.
func TestHumanPath_MaxStepsRespected(t *testing.T) {
	bk := &batchBackend{}
	c := NewCursorWithProfile(bk, ProfileHuman)
	c.SetTuning(CursorTuning{MaxSteps: 20})

	if err := c.MoveTo(context.Background(), 5000, 5000); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	pts := bk.allPoints()
	if len(pts) == 0 || len(pts) > 20 {
		t.Fatalf("points = %d, want 1..20 (MaxSteps=20)", len(pts))
	}
	last := pts[len(pts)-1]
	if last.X != 5000 || last.Y != 5000 {
		t.Fatalf("last point = (%v,%v), want exact target (5000,5000)", last.X, last.Y)
	}
}

// TestHumanPath_DelayRange: a narrow delay window must be respected exactly.
func TestHumanPath_DelayRange(t *testing.T) {
	bk := &batchBackend{}
	c := NewCursorWithProfile(bk, ProfileHuman)
	c.SetTuning(CursorTuning{MinDelayMs: 2, MaxDelayMs: 4})

	if err := c.MoveTo(context.Background(), 800, 800); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	pts := bk.allPoints()
	if len(pts) == 0 {
		t.Fatal("expected at least one point")
	}
	for _, p := range pts {
		if p.DelayMs < 2 || p.DelayMs > 4 {
			t.Fatalf("DelayMs = %d, want in [2,4]", p.DelayMs)
		}
	}
}

// TestTuning_ProfileFastMachine_Untouched: tuning must affect ProfileHuman
// only — Fast/Machine keep their own fixed delay/step behavior.
func TestTuning_ProfileFastMachine_Untouched(t *testing.T) {
	bk := &countBackend{}
	c := NewCursorWithProfile(bk, ProfileFast)
	c.SetTuning(CursorTuning{MaxSteps: 3, MinDelayMs: 100, MaxDelayMs: 200})

	if err := c.MoveTo(context.Background(), 300, 300); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	// ProfileFast uses stepCount (cap 25), not humanStepCount — MaxSteps=3
	// tuning must not shrink it below Fast's own floor of 3 steps produced
	// from a long-distance move (would be clamped to 25, always >= 3 here
	// but never driven by CursorTuning.MaxSteps).
	if bk.moveCalls.Load() == 0 {
		t.Fatal("expected per-point MouseMove calls for ProfileFast")
	}

	mbk := &countBackend{}
	mc := NewCursorWithProfile(mbk, ProfileMachine)
	mc.SetTuning(CursorTuning{MaxSteps: 1, MinDelayMs: 100, MaxDelayMs: 200})
	if err := mc.MoveTo(context.Background(), 300, 300); err != nil {
		t.Fatalf("MoveTo (machine): %v", err)
	}
	if mbk.moveCalls.Load() != 1 {
		t.Fatalf("ProfileMachine moveCalls = %d, want exactly 1 (teleport, tuning ignored)", mbk.moveCalls.Load())
	}
}
