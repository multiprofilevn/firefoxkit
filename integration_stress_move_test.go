//go:build stress

// Package stress suite (move latency + growth) — companion to
// integration_stress_test.go. Answers the open architecture question: does
// Firefox serialize input.performActions across browsingContexts, or run
// them in parallel? Run manually on a dev machine with a real Firefox 129+.
//
//	FIREFOXKIT_FIREFOX_PATH=/path/to/firefox go test -tags stress -run StressMove -v -timeout 20m
//
// Thresholds are relative + warmed up so they tolerate machine noise; every
// test skips when FIREFOXKIT_FIREFOX_PATH is unset. Test 3 (K pages
// concurrent) is a discovery probe — it logs a verdict but does not hard-
// assert the ratio, per phase-05 spec.
package firefoxkit_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/tuwibu/firefoxkit"
	"github.com/tuwibu/firefoxkit/input"
)

// fixedCoords are used across probes so layout/navigation noise does not
// contaminate latency measurements — every move targets the same two points
// on about:blank.
var fixedCoordsA = [2]float64{100, 100}
var fixedCoordsB = [2]float64{400, 300}

// latencyStats holds p50/p95/max over a set of durations.
type latencyStats struct {
	p50, p95, max time.Duration
	n             int
}

func computeStats(durs []time.Duration) latencyStats {
	if len(durs) == 0 {
		return latencyStats{}
	}
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pct := func(p float64) time.Duration {
		idx := int(p * float64(len(sorted)-1))
		return sorted[idx]
	}
	return latencyStats{
		p50: pct(0.50),
		p95: pct(0.95),
		max: sorted[len(sorted)-1],
		n:   len(sorted),
	}
}

// measureMoves times N sequential MoveTo calls alternating between two fixed
// points, discarding the first `warmup` samples.
func measureMoves(t *testing.T, m *firefoxkit.MouseAPI, n, warmup int) []time.Duration {
	t.Helper()
	durs := make([]time.Duration, 0, n)
	for i := 0; i < warmup+n; i++ {
		x, y := fixedCoordsA[0], fixedCoordsA[1]
		if i%2 == 1 {
			x, y = fixedCoordsB[0], fixedCoordsB[1]
		}
		start := time.Now()
		if err := m.MoveTo(x, y); err != nil {
			t.Fatalf("MoveTo iter %d: %v", i, err)
		}
		el := time.Since(start)
		if i >= warmup {
			durs = append(durs, el)
		}
	}
	return durs
}

// TestStress_MoveLatency_ByProfile: single page, sequential MoveTo, compares
// p50/p95/max latency across CursorHuman / CursorFast / CursorMachine.
func TestStress_MoveLatency_ByProfile(t *testing.T) {
	ff := stressFirefox(t)
	const n = 30
	const warmup = 5

	profiles := []struct {
		name    string
		profile firefoxkit.CursorProfile
	}{
		{"Human", firefoxkit.CursorHuman},
		{"Fast", firefoxkit.CursorFast},
		{"Machine", firefoxkit.CursorMachine},
	}

	results := make(map[string]latencyStats, len(profiles))
	for _, pr := range profiles {
		func() {
			b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true), firefoxkit.WithCursorProfile(pr.profile))
			if err != nil {
				t.Fatalf("launch (%s): %v", pr.name, err)
			}
			defer b.Close()
			p := b.Current()
			if err := p.Navigate("about:blank", 15*time.Second); err != nil {
				t.Fatalf("navigate (%s): %v", pr.name, err)
			}
			durs := measureMoves(t, p.Mouse(), n, warmup)
			results[pr.name] = computeStats(durs)
		}()
	}

	t.Logf("MoveTo latency by profile (n=%d, warmup=%d):", n, warmup)
	for _, pr := range profiles {
		s := results[pr.name]
		t.Logf("  %-8s p50=%v p95=%v max=%v", pr.name, s.p50, s.p95, s.max)
	}

	// Loose relative assertion: Machine should not be slower than Human, and
	// Fast should not be slower than Human either. Machine vs Fast ordering
	// is not asserted (both can be near-teleport and dominated by RPC noise).
	human, fast, machine := results["Human"].p50, results["Fast"].p50, results["Machine"].p50
	if machine > human {
		t.Errorf("expected CursorMachine p50 (%v) <= CursorHuman p50 (%v)", machine, human)
	}
	if fast > human {
		t.Errorf("expected CursorFast p50 (%v) <= CursorHuman p50 (%v)", fast, human)
	}
}

// TestStress_MoveLatency_SamePage_KGoroutines: K goroutines call MoveTo
// concurrently on the SAME page. MouseAPI serializes gestures on the cursor
// engine (per-cursor state), so total wall-clock should be roughly K times a
// single move — proving concurrent callers on one page are NOT free.
func TestStress_MoveLatency_SamePage_KGoroutines(t *testing.T) {
	ff := stressFirefox(t)
	const k = 20

	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true), firefoxkit.WithCursorProfile(firefoxkit.CursorFast))
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.Close()
	p := b.Current()
	if err := p.Navigate("about:blank", 15*time.Second); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	m := p.Mouse()

	// Warm up + measure a single move baseline.
	single := measureMoves(t, m, 10, 3)
	singleAvg := avgDuration(single)

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < k; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			x, y := fixedCoordsA[0], fixedCoordsA[1]
			if i%2 == 1 {
				x, y = fixedCoordsB[0], fixedCoordsB[1]
			}
			if err := m.MoveTo(x, y); err != nil {
				t.Errorf("goroutine %d MoveTo: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	total := time.Since(start)

	ratio := float64(total) / float64(singleAvg)
	t.Logf("same-page K=%d goroutines: single-move avg=%v total=%v ratio=%.2fx (expect ~%dx if serialized)",
		k, singleAvg, total, ratio, k)

	// Loose bound: serialization means ratio should be well above 1x (not
	// parallel) — assert it's at least K/2 to tolerate scheduling jitter.
	if ratio < float64(k)/2 {
		t.Errorf("expected same-page concurrent moves to serialize (ratio>=%.1f), got ratio=%.2f", float64(k)/2, ratio)
	}
}

func avgDuration(durs []time.Duration) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durs {
		sum += d
	}
	return sum / time.Duration(len(durs))
}

// TestStress_MoveLatency_KPages_Concurrent is the serialization probe: K
// pages in ONE browser, each fires a single MoveTo concurrently. This
// answers whether Firefox's BiDi input.performActions serializes across
// browsingContexts (ratio≈K) or runs them in parallel (ratio≈1). Discovery
// test — logs a clear verdict, does not hard-assert the ratio, only that it
// completes without error (per phase-05 spec).
func TestStress_MoveLatency_KPages_Concurrent(t *testing.T) {
	ff := stressFirefox(t)
	const k = 20 // representative sample of the 200-process fleet, NOT the full 200

	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true), firefoxkit.WithCursorProfile(firefoxkit.CursorFast))
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.Close()

	pages := make([]*firefoxkit.Page, 0, k)
	// Reuse the initial page as page 0.
	pages = append(pages, b.Current())
	for i := 1; i < k; i++ {
		p, err := b.NewPage()
		if err != nil {
			t.Fatalf("NewPage %d: %v", i, err)
		}
		pages = append(pages, p)
	}
	for i, p := range pages {
		if err := p.Navigate("about:blank", 15*time.Second); err != nil {
			t.Fatalf("navigate page %d: %v", i, err)
		}
	}

	// Warm up + single-move baseline on page 0 (isolated, sequential).
	single := measureMoves(t, pages[0].Mouse(), 10, 3)
	singleAvg := avgDuration(single)

	var wg sync.WaitGroup
	start := time.Now()
	for i, p := range pages {
		wg.Add(1)
		go func(i int, p *firefoxkit.Page) {
			defer wg.Done()
			x, y := fixedCoordsA[0], fixedCoordsA[1]
			if i%2 == 1 {
				x, y = fixedCoordsB[0], fixedCoordsB[1]
			}
			if err := p.Mouse().MoveTo(x, y); err != nil {
				t.Errorf("page %d MoveTo: %v", i, err)
			}
		}(i, p)
	}
	wg.Wait()
	total := time.Since(start)

	ratio := float64(total) / float64(singleAvg)
	verdict := "PARALLEL (ratio≈1)"
	if ratio > float64(k)/2 {
		verdict = fmt.Sprintf("SERIALIZED (ratio≈%d)", k)
	} else if ratio > 2 {
		verdict = "PARTIALLY SERIALIZED (ratio between 1x and Kx)"
	}
	t.Logf("K=%d pages concurrent move (representative sample vs 200-process fleet): single-move avg=%v total=%v ratio=%.2fx",
		k, singleAvg, total, ratio)
	t.Logf("VERDICT: Firefox performActions across browsingContexts: %s", verdict)
}

// TestStress_ChurnGrowth_NoLeak: T rounds of {open tab -> MoveTo -> close
// tab}; samples goroutines + len(Pages()) + RSS at intervals; asserts
// goroutine delta stays small, Pages() returns to 1, RSS does not grow
// linearly. Uses WithCursorTuning to keep MoveTo fast (small step cap).
func TestStress_ChurnGrowth_NoLeak(t *testing.T) {
	ff := stressFirefox(t)
	const rounds = 200 // representative sample of a long-running fleet process, not literal 200 target
	const sampleEvery = 20

	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true),
		firefoxkit.WithCursorProfile(firefoxkit.CursorFast),
		firefoxkit.WithCursorTuning(input.CursorTuning{MaxSteps: 12}))
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.Close()

	base := b.Current()
	if err := base.Navigate("about:blank", 15*time.Second); err != nil {
		t.Fatalf("navigate base: %v", err)
	}

	baselinePages := len(b.Pages())
	beforeGoroutines := settleGoroutines()
	beforeRSS := rssKB(b.PID())

	type sample struct {
		round      int
		goroutines int
		pages      int
		rssKB      int64
	}
	samples := make([]sample, 0, rounds/sampleEvery+1)

	for i := 0; i < rounds; i++ {
		p, err := b.NewPage()
		if err != nil {
			t.Fatalf("NewPage round %d: %v", i, err)
		}
		if err := p.Navigate("about:blank", 15*time.Second); err != nil {
			t.Fatalf("navigate round %d: %v", i, err)
		}
		if err := p.Mouse().MoveTo(fixedCoordsA[0], fixedCoordsA[1]); err != nil {
			t.Fatalf("MoveTo round %d: %v", i, err)
		}
		p.Close()

		if (i+1)%sampleEvery == 0 {
			g := settleGoroutines()
			rss := rssKB(b.PID())
			samples = append(samples, sample{round: i + 1, goroutines: g, pages: len(b.Pages()), rssKB: rss})
		}
	}

	afterGoroutines := settleGoroutines()
	afterPages := len(b.Pages())
	afterRSS := rssKB(b.PID())

	t.Logf("churn growth over %d rounds (baseline pages=%d, before goroutines=%d, before RSS=%dKB):",
		rounds, baselinePages, beforeGoroutines, beforeRSS)
	for _, s := range samples {
		t.Logf("  round=%-4d goroutines=%-4d pages=%-3d rssKB=%d", s.round, s.goroutines, s.pages, s.rssKB)
	}
	t.Logf("after: goroutines=%d pages=%d rssKB=%d", afterGoroutines, afterPages, afterRSS)

	if afterPages != baselinePages {
		t.Fatalf("page leak after churn: baseline=%d after=%d", baselinePages, afterPages)
	}
	if afterGoroutines > beforeGoroutines+5 {
		t.Fatalf("goroutine leak after churn: before=%d after=%d (delta=%d)", beforeGoroutines, afterGoroutines, afterGoroutines-beforeGoroutines)
	}
	if beforeRSS > 0 && afterRSS > 0 {
		// Generous headroom for GC/allocator jitter; a real per-tab leak over
		// `rounds` iterations would blow well past +100%.
		if afterRSS > beforeRSS*2 {
			t.Errorf("possible RSS growth: before=%dKB after=%dKB (+%.0f%%)",
				beforeRSS, afterRSS, float64(afterRSS-beforeRSS)/float64(beforeRSS)*100)
		}
	} else {
		t.Log("RSS unavailable on this platform; skipped RSS growth assertion")
	}
}
