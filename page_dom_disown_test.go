package firefoxkit

import (
	"encoding/json"
	"testing"
	"time"
)

// callFunctionResult builds a script.callFunction success payload whose
// RemoteValue is the given value.
func callFunctionResult(remoteValue any) any {
	return map[string]any{"type": "success", "result": remoteValue}
}

// waitDisown polls until script.disown has been called n times (the disown is
// best-effort and may run after QuerySelectorAll returns).
func waitDisown(t *testing.T, m *mockBiDi, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.callCount("script.disown") >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("script.disown calls = %d, want >= %d", m.callCount("script.disown"), n)
}

// TestQuerySelectorAll_DisownsArrayHandle locks in the H1 fix: the top-level
// array RemoteValue is created with ownership=root and must be disowned after
// the element handles are extracted — otherwise Firefox pins the array (and
// transitively every node) forever.
func TestQuerySelectorAll_DisownsArrayHandle(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("script.callFunction", func(json.RawMessage) (any, string) {
		return callFunctionResult(map[string]any{
			"type":   "array",
			"handle": "h-arr",
			"value": []any{
				map[string]any{"type": "node", "handle": "h1"},
				map[string]any{"type": "node", "handle": "h2"},
			},
		}), ""
	})

	els, err := p.QuerySelectorAll("div", time.Second)
	if err != nil {
		t.Fatalf("QuerySelectorAll: %v", err)
	}
	if len(els) != 2 || els[0].handle != "h1" || els[1].handle != "h2" {
		t.Fatalf("unexpected handles: %+v", els)
	}

	waitDisown(t, m, 1)
	var dp struct {
		Handles []string `json:"handles"`
	}
	if err := json.Unmarshal(m.lastParams("script.disown"), &dp); err != nil {
		t.Fatalf("decode disown params: %v", err)
	}
	if len(dp.Handles) != 1 || dp.Handles[0] != "h-arr" {
		t.Fatalf("disowned handles = %v, want [h-arr] only (element handles belong to caller)", dp.Handles)
	}
}

// TestQuerySelectorAll_ExceptionResult: a thrown selector error must surface as
// an error, not silently return an empty slice.
func TestQuerySelectorAll_ExceptionResult(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("script.callFunction", func(json.RawMessage) (any, string) {
		return map[string]any{
			"type":             "exception",
			"exceptionDetails": map[string]any{"text": "SyntaxError: bad selector"},
		}, ""
	})

	_, err := p.QuerySelectorAll("!!!", time.Second)
	if err == nil {
		t.Fatal("expected error for exception result, got nil")
	}
}

// TestQuerySelectorAll_EmptyResult: empty match list → empty slice, and no
// stray disown call when the array carries no handle.
func TestQuerySelectorAll_EmptyResult(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("script.callFunction", func(json.RawMessage) (any, string) {
		return callFunctionResult(map[string]any{"type": "array", "value": []any{}}), ""
	})

	els, err := p.QuerySelectorAll("div", time.Second)
	if err != nil {
		t.Fatalf("QuerySelectorAll: %v", err)
	}
	if len(els) != 0 {
		t.Fatalf("expected empty slice, got %d handles", len(els))
	}
	time.Sleep(50 * time.Millisecond)
	if got := m.callCount("script.disown"); got != 0 {
		t.Fatalf("script.disown calls = %d, want 0 (no top-level handle)", got)
	}
}

// TestQuerySelector_UnmarshalError_Disowns: L1 — when the single-element result
// has a handle but decoding fails, the handle must still be released.
func TestQuerySelector_UnmarshalError_Disowns(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("script.callFunction", func(json.RawMessage) (any, string) {
		// Handle present but "sharedId" is a number → unmarshal into string fails.
		return callFunctionResult(map[string]any{
			"type":     "node",
			"handle":   "h-bad",
			"sharedId": 12345,
		}), ""
	})

	_, err := p.QuerySelector("div", time.Second)
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
	waitDisown(t, m, 1)
}
