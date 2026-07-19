package firefoxkit

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func pollUntilTrue(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// addMockPage appends a second page to the mock browser.
func addMockPage(b *Browser, id string) *Page {
	p := &Page{browser: b, contextID: id}
	b.pagesMu.Lock()
	b.pages = append(b.pages, p)
	b.pagesMu.Unlock()
	return p
}

// TestContextDestroyed_RemovesPage locks in the H3 fix: when Firefox destroys
// a browsing context (window.close(), self-closing popup), the page must be
// dropped from b.pages and cleaned up — previously it leaked forever.
func TestContextDestroyed_RemovesPage(t *testing.T) {
	b, p1, m := newMockBrowser(t)
	p2 := addMockPage(b, "ctx-2")

	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}

	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{
		"context": "ctx-2", "url": "about:blank",
	})

	if !pollUntilTrue(t, 2*time.Second, func() bool { return len(b.Pages()) == 1 }) {
		t.Fatalf("pages = %d, want 1 (destroyed context not removed)", len(b.Pages()))
	}
	if b.Current() != p1 {
		t.Fatal("current must remain the surviving page")
	}
	if !p2.cleanedUp.Load() {
		t.Fatal("destroyed page was not cleaned up")
	}
}

// TestContextDestroyed_CurrentPage_SwitchesCurrent: destroying the current
// page must move b.current to a survivor.
func TestContextDestroyed_CurrentPage_SwitchesCurrent(t *testing.T) {
	b, p1, m := newMockBrowser(t)
	p2 := addMockPage(b, "ctx-2")
	b.pagesMu.Lock()
	b.current = p2
	b.pagesMu.Unlock()

	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": "ctx-2"})

	if !pollUntilTrue(t, 2*time.Second, func() bool { return b.Current() == p1 }) {
		t.Fatal("current not switched to surviving page")
	}
}

// TestContextDestroyed_UnknownContext_Noop: an event for an untracked context
// must not disturb the page list.
func TestContextDestroyed_UnknownContext_Noop(t *testing.T) {
	b, _, m := newMockBrowser(t)
	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": "ctx-nope"})
	time.Sleep(100 * time.Millisecond)
	if got := len(b.Pages()); got != 1 {
		t.Fatalf("pages = %d, want 1", got)
	}
}

// TestPageCleanup_UnsubscribesServerSide locks in the M2 fix: cleanup must
// issue session.unsubscribe for the per-context events this page subscribed,
// so server-side subscriptions don't accumulate across page churn.
func TestPageCleanup_UnsubscribesServerSide(t *testing.T) {
	_, p, m := newMockBrowser(t)

	p.OnConsole(func(ConsoleEvent) {})
	p.OnDialog(func(DialogEvent) DialogAction { return DialogAction{} })

	p.cleanup(context.Background())

	if got := m.callCount("session.unsubscribe"); got == 0 {
		t.Fatal("cleanup must call session.unsubscribe")
	}
	var up struct {
		Events   []string `json:"events"`
		Contexts []string `json:"contexts"`
	}
	if err := json.Unmarshal(m.lastParams("session.unsubscribe"), &up); err != nil {
		t.Fatalf("decode unsubscribe params: %v", err)
	}
	want := map[string]bool{"log.entryAdded": false, "browsingContext.userPromptOpened": false}
	for _, e := range up.Events {
		if _, ok := want[e]; ok {
			want[e] = true
		}
	}
	for e, seen := range want {
		if !seen {
			t.Errorf("event %q missing from session.unsubscribe", e)
		}
	}
	if len(up.Contexts) != 1 || up.Contexts[0] != "ctx-1" {
		t.Errorf("contexts = %v, want [ctx-1]", up.Contexts)
	}
}

// TestPageCleanup_Idempotent: double cleanup (contextDestroyed + Browser.Close)
// must unsubscribe exactly once.
func TestPageCleanup_Idempotent(t *testing.T) {
	_, p, m := newMockBrowser(t)
	p.OnConsole(func(ConsoleEvent) {})

	p.cleanup(context.Background())
	p.cleanup(context.Background())

	if got := m.callCount("session.unsubscribe"); got != 1 {
		t.Fatalf("session.unsubscribe calls = %d, want 1", got)
	}
}
