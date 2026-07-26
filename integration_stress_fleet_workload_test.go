//go:build stress

// Mixed-workload fleet stress — N SEPARATE Firefox processes each running a
// realistic automation loop (navigate → evaluate → click → type → tab churn →
// occasional screenshot) while a sampler tracks goroutines, Go heap, and total
// Firefox RSS over time. Companion to integration_stress_move_test.go (which
// only measures MoveTo latency).
//
//	FIREFOXKIT_FIREFOX_PATH=... FIREFOXKIT_FLEET_N=100 FIREFOXKIT_HEADED=1 \
//	  go test -tags stress -race -run TestStress_FleetWorkload -v -timeout 60m
//
// Knobs: FIREFOXKIT_FLEET_N (default 25), FIREFOXKIT_HEADED=1 (default
// headless), FIREFOXKIT_WORKLOAD_SECONDS (default 180). A safety valve aborts
// the whole run if available physical memory drops below 8 GB.
package firefoxkit_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tuwibu/firefoxkit"
)

// workloadNavTimeout bounds a single Navigate under heavy fleet load.
// Overridable via FIREFOXKIT_NAV_TIMEOUT_SECONDS: the launch stampede of a
// large headed fleet produces oversubscription latency (not a leak), so
// verify runs use a wider bound to keep that noise out of the error count.
var workloadNavTimeout = 20 * time.Second

const (
	// samplerInterval is how often the sampler logs goroutine/heap/RSS.
	samplerInterval = 15 * time.Second
	// minFreePhysKB is the safety valve: abort when available physical
	// memory drops below this (8 GB in KB).
	minFreePhysKB = 8 * 1024 * 1024
	// maxGoroutineDelta is the allowed goroutine growth after the whole
	// fleet is closed and the runtime has settled.
	maxGoroutineDelta = 20
)

func headedFromEnv() bool { return os.Getenv("FIREFOXKIT_HEADED") == "1" }

func workloadDuration(t *testing.T) time.Duration {
	t.Helper()
	v := os.Getenv("FIREFOXKIT_WORKLOAD_SECONDS")
	if v == "" {
		return 180 * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		t.Fatalf("FIREFOXKIT_WORKLOAD_SECONDS=%q invalid: %v", v, err)
	}
	return time.Duration(n) * time.Second
}

// workloadServer serves three tiny pages (button + input) over localhost so
// navigation does not depend on Firefox's data:-URI top-level policy.
func workloadServer() *httptest.Server {
	page := func(name string) string {
		return fmt.Sprintf(`<html><head><title>%s</title></head><body><h1>%s</h1>
<button id="b" onclick="this.dataset.n=(+this.dataset.n||0)+1">go</button>
<input id="i"></body></html>`, name, name)
	}
	mux := http.NewServeMux()
	for _, name := range []string{"a", "b", "c"} {
		body := page("wl-" + name)
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux)
}

// freePhysMemKB returns available physical memory in KB, or 0 when the probe
// is unavailable (valve then disabled — logged once by the sampler).
func freePhysMemKB() int64 {
	out, err := exec.Command("wmic", "OS", "get", "FreePhysicalMemory", "/Value").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "FreePhysicalMemory="); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return 0
			}
			return n
		}
	}
	return 0
}

// allFirefoxRSSKB sums RSS across every firefox.exe on the machine — fleet
// child processes included, which Browser.PID (parent only) misses. Unrelated
// firefox processes inflate this; the caller logs the pre-run count as noise
// context. Returns 0 when tasklist is unavailable.
func allFirefoxRSSKB() int64 {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq firefox.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, "\",\"")
		if len(fields) < 5 {
			continue
		}
		mem := strings.TrimRight(strings.TrimSpace(fields[4]), "\" K")
		mem = strings.ReplaceAll(mem, ",", "")
		mem = strings.ReplaceAll(mem, ".", "")
		if n, err := strconv.ParseInt(strings.TrimSpace(mem), 10, 64); err == nil {
			total += n
		}
	}
	return total
}

// countFirefoxProcs returns how many firefox.exe processes exist machine-wide
// (fleet children included; 0 when tasklist is unavailable). Compared
// before-launch vs after-close to catch orphaned browser processes.
func countFirefoxProcs() int {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq firefox.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(line, "firefox.exe") {
			n++
		}
	}
	return n
}

// fleetRSSKB sums resident set size across every fleet browser PID.
func fleetRSSKB(fleet []*firefoxkit.Browser) int64 {
	var total int64
	for _, b := range fleet {
		if pid := b.PID(); pid != 0 {
			total += rssKB(pid)
		}
	}
	return total
}

// runWorkloadLoop drives one browser through the mixed workload until the
// deadline passes or abort is set. Any error is counted, reported, and ends
// this browser's loop (loud, never swallowed). Returns iterations completed.
func runWorkloadLoop(t *testing.T, idx int, b *firefoxkit.Browser, baseURL string, deadline time.Time, abort *atomic.Bool, errCount *atomic.Int64) int {
	rng := rand.New(rand.NewSource(int64(idx)*7919 + 17)) // per-goroutine: rand.Rand is not goroutine-safe
	paths := [3]string{"/a", "/b", "/c"}
	p := b.Current()
	fail := func(step string, err error) {
		errCount.Add(1)
		t.Errorf("browser %d iter step %s: %v", idx, step, err)
	}
	iters := 0
	for time.Now().Before(deadline) && !abort.Load() {
		url := baseURL + paths[iters%3]
		if err := p.Navigate(url, workloadNavTimeout); err != nil {
			fail("navigate", err)
			return iters
		}
		var title string
		if err := p.Evaluate("document.title", &title); err != nil {
			fail("evaluate", err)
			return iters
		}
		if err := p.Click("#b"); err != nil {
			fail("click", err)
			return iters
		}
		if err := p.Type("#i", "stress"); err != nil {
			fail("type", err)
			return iters
		}
		if iters%5 == 4 { // tab churn every 5th iteration
			np, err := b.NewPage()
			if err != nil {
				fail("newpage", err)
				return iters
			}
			if err := np.Navigate(url, workloadNavTimeout); err != nil {
				fail("newpage-navigate", err)
				return iters
			}
			if err := b.ClosePage(np.ID()); err != nil {
				fail("closepage", err)
				return iters
			}
		}
		if rng.Intn(10) == 0 {
			if _, err := p.Screenshot(); err != nil {
				fail("screenshot", err)
				return iters
			}
		}
		iters++
	}
	return iters
}

// TestStress_FleetWorkload: N separate Firefox processes run the mixed
// workload concurrently. Asserts zero workload errors, bounded goroutine
// delta after full close, and bounded Go heap growth. Firefox RSS is logged
// for discovery (no hard assertion on the first run).
func TestStress_FleetWorkload(t *testing.T) {
	ff := stressFirefox(t)
	n := fleetSize(t)
	headless := !headedFromEnv()
	dur := workloadDuration(t)
	if v := os.Getenv("FIREFOXKIT_NAV_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("FIREFOXKIT_NAV_TIMEOUT_SECONDS=%q invalid: %v", v, err)
		}
		workloadNavTimeout = time.Duration(n) * time.Second
	}
	t.Logf("fleet workload: N=%d headless=%v duration=%v", n, headless, dur)

	srv := workloadServer()
	defer srv.Close()

	beforeGoroutines := settleGoroutines()
	ffProcsBefore := countFirefoxProcs()

	fleet := launchFleet(t, ff, n, firefoxkit.CursorFast, headless)
	var closeOnce sync.Once
	closeFleet := func() {
		closeOnce.Do(func() {
			for i, b := range fleet {
				if err := b.Close(); err != nil {
					t.Logf("close browser %d: %v", i, err)
				}
			}
		})
	}
	defer closeFleet()

	var msLaunch runtime.MemStats
	runtime.ReadMemStats(&msLaunch)
	t.Logf("post-launch: goroutines=%d heapAllocMB=%d fleetParentRSSMB=%d allFirefoxRSSMB=%d freePhysMB=%d",
		runtime.NumGoroutine(), msLaunch.HeapAlloc>>20, fleetRSSKB(fleet)>>10, allFirefoxRSSKB()>>10, freePhysMemKB()>>10)

	// Sampler + safety valve.
	var abort atomic.Bool
	stopSampler := make(chan struct{})
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		start := time.Now()
		valveWarned := false
		tick := time.NewTicker(samplerInterval)
		defer tick.Stop()
		for {
			select {
			case <-stopSampler:
				return
			case <-tick.C:
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				free := freePhysMemKB()
				// Single tasklist call per sample — the per-PID fleetRSSKB loop
				// spawns N processes and starved the sampler for minutes at N=100.
				t.Logf("sample t=%v goroutines=%d heapAllocMB=%d sysMB=%d allFirefoxRSSMB=%d freePhysMB=%d",
					time.Since(start).Round(time.Second), runtime.NumGoroutine(),
					ms.HeapAlloc>>20, ms.Sys>>20, allFirefoxRSSKB()>>10, free>>10)
				if free == 0 && !valveWarned {
					valveWarned = true
					t.Log("free-memory probe unavailable; safety valve disabled")
				}
				if free > 0 && free < minFreePhysKB {
					t.Errorf("SAFETY VALVE: free physical memory %dMB < %dMB — aborting workload", free>>10, int64(minFreePhysKB)>>10)
					abort.Store(true)
					return
				}
			}
		}
	}()

	// Workers: one goroutine per browser.
	deadline := time.Now().Add(dur)
	var wg sync.WaitGroup
	var errCount, iterTotal atomic.Int64
	for i, b := range fleet {
		wg.Add(1)
		go func(i int, b *firefoxkit.Browser) {
			defer wg.Done()
			iterTotal.Add(int64(runWorkloadLoop(t, i, b, srv.URL, deadline, &abort, &errCount)))
		}(i, b)
	}
	wg.Wait()
	close(stopSampler)
	<-samplerDone

	closeFleet()
	afterGoroutines := settleGoroutines()
	var msAfter runtime.MemStats
	runtime.ReadMemStats(&msAfter)
	t.Logf("SUMMARY: iters=%d errors=%d goroutines before=%d after=%d heapAllocMB post-launch=%d post-close=%d",
		iterTotal.Load(), errCount.Load(), beforeGoroutines, afterGoroutines,
		msLaunch.HeapAlloc>>20, msAfter.HeapAlloc>>20)

	if errCount.Load() != 0 {
		t.Errorf("workload finished with %d errors (see log)", errCount.Load())
	}
	if delta := afterGoroutines - beforeGoroutines; delta > maxGoroutineDelta {
		t.Errorf("goroutine leak after fleet close: before=%d after=%d (delta=%d > %d)",
			beforeGoroutines, afterGoroutines, delta, maxGoroutineDelta)
		var buf bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&buf, 1)
		t.Logf("leaked goroutine profile (aggregated by stack):\n%s", buf.String())
	}
	// Orphan check: every firefox process the fleet spawned must be gone.
	// Slow shutdowns get a short grace window before we call it a leak.
	ffProcsAfter := countFirefoxProcs()
	if ffProcsAfter > ffProcsBefore {
		time.Sleep(5 * time.Second)
		ffProcsAfter = countFirefoxProcs()
	}
	if ffProcsAfter > ffProcsBefore {
		t.Errorf("orphan firefox processes after fleet close: before=%d after=%d", ffProcsBefore, ffProcsAfter)
	}
	// Post-close heap should return near the post-launch level; 2x + 64MB
	// slack tolerates GC/allocator jitter while still catching linear leaks.
	if limit := msLaunch.HeapAlloc*2 + 64<<20; msAfter.HeapAlloc > limit {
		t.Errorf("possible Go heap leak: post-close HeapAlloc=%dMB > limit=%dMB (2x post-launch + 64MB)",
			msAfter.HeapAlloc>>20, limit>>20)
	}
}
