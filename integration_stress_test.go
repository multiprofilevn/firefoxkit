//go:build stress

// Package stress suite — run manually on a dev machine with a real Firefox
// 129+. NOT part of the default `go test` run (build tag `stress`).
//
//	FIREFOXKIT_FIREFOX_PATH=/path/to/firefox go test -tags stress -race -run Stress -v -timeout 20m
//
// Each test asserts a leak/perf property fixed in plan 260719-1620. Thresholds
// are relative + warmed up so they tolerate machine noise; every test skips
// when FIREFOXKIT_FIREFOX_PATH is unset.
package firefoxkit_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/multiprofilevn/firefoxkit"
)

func stressFirefox(t *testing.T) string {
	p := os.Getenv("FIREFOXKIT_FIREFOX_PATH")
	if p == "" {
		t.Skip("FIREFOXKIT_FIREFOX_PATH not set; skipping stress suite")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("firefox binary missing at %s: %v", p, err)
	}
	return p
}

// rssKB returns the resident set size of a pid in KB (Windows: tasklist).
// Returns 0 (and skips the numeric assertion at the call site) if unavailable.
func rssKB(pid uint32) int64 {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "\",\"")
	if len(fields) < 5 {
		return 0
	}
	mem := strings.TrimRight(fields[4], "\" K")
	mem = strings.ReplaceAll(mem, ",", "")
	mem = strings.ReplaceAll(mem, ".", "")
	n, err := strconv.ParseInt(strings.TrimSpace(mem), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func settleGoroutines() int {
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	return runtime.NumGoroutine()
}

// TestStress_GoroutineLeak_LaunchClose: repeated Launch→listeners→Close must
// not leak goroutines.
func TestStress_GoroutineLeak_LaunchClose(t *testing.T) {
	ff := stressFirefox(t)
	const n = 20

	// Warm up once so one-time globals don't count as a leak.
	b0, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("warmup launch: %v", err)
	}
	_ = b0.Close()
	before := settleGoroutines()

	for i := 0; i < n; i++ {
		b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
		if err != nil {
			t.Fatalf("launch %d: %v", i, err)
		}
		p := b.Current()
		p.OnConsole(func(firefoxkit.ConsoleEvent) {})
		p.OnDialog(func(firefoxkit.DialogEvent) firefoxkit.DialogAction { return firefoxkit.DialogAction{} })
		_ = p.Navigate("about:blank", 15*time.Second)
		_ = b.Close()
	}

	after := settleGoroutines()
	if after > before+3 {
		t.Fatalf("goroutine leak: before=%d after=%d (delta=%d)", before, after, after-before)
	}
}

// TestStress_HandleLeak_QuerySelectorAll: 1000 QuerySelectorAll calls must not
// grow the Firefox-side RSS unbounded (H1 — array handles now disowned).
func TestStress_HandleLeak_QuerySelectorAll(t *testing.T) {
	ff := stressFirefox(t)
	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.Close()
	p := b.Current()

	// A page with a few hundred nodes so each call pins a non-trivial array.
	html := "data:text/html," + strings.Repeat("<div class=x>a</div>", 300)
	if err := p.Navigate(html, 15*time.Second); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	for i := 0; i < 100; i++ { // warmup
		_, _ = p.QuerySelectorAll("div.x", 5*time.Second)
	}
	time.Sleep(500 * time.Millisecond)
	base := rssKB(b.PID())
	if base == 0 {
		t.Skip("RSS unavailable on this platform")
	}

	for i := 0; i < 1000; i++ {
		if _, err := p.QuerySelectorAll("div.x", 5*time.Second); err != nil {
			t.Fatalf("QuerySelectorAll %d: %v", i, err)
		}
	}
	time.Sleep(1 * time.Second)
	after := rssKB(b.PID())

	// Allow generous headroom for GC jitter; a leak of 300 arrays * 1000 calls
	// would blow well past +50%.
	if after > base*3/2 {
		t.Fatalf("possible handle leak: RSS base=%dKB after=%dKB (+%.0f%%)",
			base, after, float64(after-base)/float64(base)*100)
	}
	t.Logf("RSS base=%dKB after=%dKB", base, after)
}

// TestStress_TabChurn_NoPageLeak: opening and self-closing many popups must
// leave the page list at baseline (H3 — contextDestroyed prunes them).
func TestStress_TabChurn_NoPageLeak(t *testing.T) {
	ff := stressFirefox(t)
	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.Close()
	p := b.Current()
	if err := p.Navigate("about:blank", 15*time.Second); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	baseline := len(b.Pages())
	before := settleGoroutines()

	for i := 0; i < 100; i++ {
		// Open a popup that immediately closes itself.
		_ = p.Evaluate(`(() => { const w = window.open("about:blank"); if (w) w.close(); return 1 })()`, new(int))
	}
	// Give contextDestroyed events time to arrive and prune.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(b.Pages()) > baseline {
		time.Sleep(100 * time.Millisecond)
	}

	if got := len(b.Pages()); got > baseline {
		t.Fatalf("page leak: baseline=%d after churn=%d", baseline, got)
	}
	after := settleGoroutines()
	if after > before+3 {
		t.Fatalf("goroutine leak after tab churn: before=%d after=%d", before, after)
	}
}

// TestStress_ConsoleFlood_BoundedGoroutines: a page spamming console must not
// grow goroutines unbounded (M3 — bounded per-subscription queue).
func TestStress_ConsoleFlood_BoundedGoroutines(t *testing.T) {
	ff := stressFirefox(t)
	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.Close()
	p := b.Current()

	// Slow handler so the queue actually backs up.
	p.OnConsole(func(firefoxkit.ConsoleEvent) { time.Sleep(time.Millisecond) })
	if err := p.Navigate("about:blank", 15*time.Second); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	before := settleGoroutines()
	// 10k console.log as fast as the page can emit.
	_ = p.Evaluate(`(() => { for (let i=0;i<10000;i++) console.log("flood",i); return 1 })()`, new(int))
	time.Sleep(3 * time.Second)
	after := settleGoroutines()

	// One worker per subscription — not one goroutine per event.
	if after > before+5 {
		t.Fatalf("console flood grew goroutines: before=%d after=%d", before, after)
	}
}

// TestStress_ProfileDir_NoLeak: Launch+Close must not leave temp profile dirs
// behind (H2).
func TestStress_ProfileDir_NoLeak(t *testing.T) {
	ff := stressFirefox(t)
	pattern := os.TempDir() + string(os.PathSeparator) + "firefoxkit-profile-*"

	countProfiles := func() int {
		matches, _ := filepath.Glob(pattern)
		return len(matches)
	}

	before := countProfiles()
	for i := 0; i < 10; i++ {
		b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
		if err != nil {
			t.Fatalf("launch %d: %v", i, err)
		}
		_ = b.Close()
	}
	// Cleanup uses a short retry loop on Windows file locks.
	time.Sleep(2 * time.Second)
	after := countProfiles()

	if after > before {
		t.Fatalf("temp profile dirs leaked: before=%d after=%d", before, after)
	}
}
