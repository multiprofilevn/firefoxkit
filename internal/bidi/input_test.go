package bidi

import (
	"encoding/json"
	"testing"
)

func TestPointerMove_Defaults(t *testing.T) {
	a := PointerMove(100, 200, 0)
	if a["type"] != "pointerMove" || a["x"].(float64) != 100 || a["y"].(float64) != 200 {
		t.Errorf("bad action: %v", a)
	}
	if a["origin"] != "viewport" {
		t.Errorf("origin = %v", a["origin"])
	}
	if _, hasDur := a["duration"]; hasDur {
		t.Errorf("duration must be omitted when 0")
	}
}

func TestPointerMove_WithDuration(t *testing.T) {
	a := PointerMove(1, 2, 250)
	if a["duration"].(int) != 250 {
		t.Errorf("duration = %v", a["duration"])
	}
}

func TestPointerDownUp(t *testing.T) {
	d := PointerDown(0)
	u := PointerUp(2)
	if d["type"] != "pointerDown" || d["button"].(int) != 0 {
		t.Errorf("down: %v", d)
	}
	if u["type"] != "pointerUp" || u["button"].(int) != 2 {
		t.Errorf("up: %v", u)
	}
}

func TestKeyDownUp(t *testing.T) {
	d := KeyDown("a")
	u := KeyUp("a")
	if d["type"] != "keyDown" || d["value"] != "a" {
		t.Errorf("keyDown: %v", d)
	}
	if u["type"] != "keyUp" || u["value"] != "a" {
		t.Errorf("keyUp: %v", u)
	}
}

func TestScroll(t *testing.T) {
	a := Scroll(10, 20, 0, 120, 0)
	if a["type"] != "scroll" || a["deltaY"].(float64) != 120 {
		t.Errorf("scroll: %v", a)
	}
}

func TestPause(t *testing.T) {
	a := Pause(50)
	if a["type"] != "pause" || a["duration"].(int) != 50 {
		t.Errorf("pause: %v", a)
	}
}

// Round-trip the params through JSON to catch struct-tag typos.
func TestPerformActionsParams_JSON(t *testing.T) {
	p := PerformActionsParams{
		Context: "ctx-1",
		Actions: []InputSource{
			{
				Type:       SourcePointer,
				ID:         "m",
				Parameters: &PointerParams{PointerType: PointerMouse},
				Actions:    []map[string]any{PointerMove(1, 2, 0), PointerDown(0), PointerUp(0)},
			},
		},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		`"context":"ctx-1"`,
		`"type":"pointer"`,
		`"pointerType":"mouse"`,
		`"pointerMove"`,
		`"pointerDown"`,
		`"pointerUp"`,
	} {
		if !contains(got, want) {
			t.Errorf("JSON missing %q in %s", want, got)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 && (haystack == needle || stringIndex(haystack, needle) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
