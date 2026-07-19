package firefoxkit

import (
	"encoding/json"
	"sync"
	"testing"
)

// failNTimes returns a session.subscribe responder that errors for the first n
// calls, then succeeds.
func failNTimes(n int) func(json.RawMessage) (any, string) {
	var mu sync.Mutex
	count := 0
	return func(json.RawMessage) (any, string) {
		mu.Lock()
		defer mu.Unlock()
		count++
		if count <= n {
			return nil, "unknown error"
		}
		return nil, ""
	}
}

// TestOnDialog_SubscribeFailure_RetriesWithoutCrash locks in the H4 fix: a
// failed session.subscribe must not fatally crash the process (the old code
// reset sync.Once inside Once.Do → "sync: unlock of unlocked mutex"), and a
// later OnDialog call must retry the subscribe.
func TestOnDialog_SubscribeFailure_RetriesWithoutCrash(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("session.subscribe", failNTimes(1))

	h := func(DialogEvent) DialogAction { return DialogAction{} }
	p.OnDialog(h) // subscribe fails — must not crash
	p.OnDialog(h) // must retry subscribe

	if got := m.callCount("session.subscribe"); got != 2 {
		t.Fatalf("session.subscribe calls = %d, want 2 (fail then retry)", got)
	}
}

// TestOnDialog_SubscribeSuccess_Idempotent: after a successful subscribe,
// further OnDialog calls only swap the handler slot — no second subscribe.
func TestOnDialog_SubscribeSuccess_Idempotent(t *testing.T) {
	_, p, m := newMockBrowser(t)

	h := func(DialogEvent) DialogAction { return DialogAction{} }
	p.OnDialog(h)
	p.OnDialog(h)
	p.OnDialog(h)

	if got := m.callCount("session.subscribe"); got != 1 {
		t.Fatalf("session.subscribe calls = %d, want 1", got)
	}
}

// TestOnConsole_SubscribeFailure_RetriesWithoutCrash mirrors the dialog test
// for the console listener (same H4 pattern).
func TestOnConsole_SubscribeFailure_RetriesWithoutCrash(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("session.subscribe", failNTimes(1))

	h := func(ConsoleEvent) {}
	p.OnConsole(h)
	p.OnConsole(h)

	if got := m.callCount("session.subscribe"); got != 2 {
		t.Fatalf("session.subscribe calls = %d, want 2 (fail then retry)", got)
	}
}

// TestOnConsole_SubscribeSuccess_Idempotent mirrors the dialog idempotency test.
func TestOnConsole_SubscribeSuccess_Idempotent(t *testing.T) {
	_, p, m := newMockBrowser(t)

	h := func(ConsoleEvent) {}
	p.OnConsole(h)
	p.OnConsole(h)
	p.OnConsole(h)

	if got := m.callCount("session.subscribe"); got != 1 {
		t.Fatalf("session.subscribe calls = %d, want 1", got)
	}
}

// TestOnDialog_ConcurrentCalls_SingleSubscribe: many goroutines racing through
// the retry path must produce exactly one successful subscription and no crash.
// Run with -race.
func TestOnDialog_ConcurrentCalls_SingleSubscribe(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("session.subscribe", failNTimes(1))

	h := func(DialogEvent) DialogAction { return DialogAction{} }
	p.OnDialog(h) // burn the failing call

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.OnDialog(h)
		}()
	}
	wg.Wait()

	// 1 failed + exactly 1 successful subscribe; later calls are no-ops.
	if got := m.callCount("session.subscribe"); got != 2 {
		t.Fatalf("session.subscribe calls = %d, want 2 (1 fail + 1 success)", got)
	}
}
