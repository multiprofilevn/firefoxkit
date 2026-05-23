package firefoxkit

import (
	"strings"
	"testing"
	"time"
)

func TestPollBackoff_Sequence(t *testing.T) {
	got := []time.Duration{}
	d := time.Duration(0)
	for i := 0; i < 6; i++ {
		d = pollBackoff(d)
		got = append(got, d)
	}
	// 50ms → 100ms → 200ms → 400ms → 500ms (capped) → 500ms
	want := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		500 * time.Millisecond,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBoundingBox_Center(t *testing.T) {
	bb := BoundingBox{X: 10, Y: 20, Width: 100, Height: 40}
	cx, cy := bb.Center()
	if cx != 60 || cy != 40 {
		t.Errorf("center = (%g, %g), want (60, 40)", cx, cy)
	}
}

func TestEvalError_Is(t *testing.T) {
	e := &evalError{cause: ErrPageNotFound}
	if !errorIs(e, ErrEvaluate) {
		t.Errorf("evalError should match ErrEvaluate")
	}
	if e.Error() == "" || !strings.Contains(e.Error(), "evaluate") {
		t.Errorf("unexpected msg: %s", e.Error())
	}
}

// errorIs avoids importing errors package twice in tests.
func errorIs(err, target error) bool {
	type isWrapper interface{ Is(error) bool }
	if w, ok := err.(isWrapper); ok {
		return w.Is(target)
	}
	return err == target
}
