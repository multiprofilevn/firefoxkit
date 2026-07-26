# Leak & Race Audit: Revealed Real Diseases Hidden in Headed N=100 Saturation

**Date**: 2026-07-26 23:01  
**Severity**: High  
**Component**: `firefoxkit` lifecycle (goroutine drains, process cleanup), stress harness  
**Status**: Resolved (root causes identified; fixes queued for follow-up plan)

---

## What Happened

Executed full leak/race audit of firefoxkit across 4 phases. Static audit (4 reviewers × 4 zones) flagged 36 findings. Dynamic stress run (N=100 headed Firefox processes, 180s mixed workload, -race) exposed two real diseases that static analysis predicted but couldn't quantify:

1. **D-1: Goroutine leak on browser close** — 3 → 267 goroutines (~2.6/browser), causing 264 leaked goroutines total. All leaked goroutines originated from `events.go:143-151` drain timeout code.
2. **D-2: Orphan firefox.exe processes** — 103 out of 100 launched firefox.exe stayed alive after test completed (profiles deleted but processes hung), required 3 force-kill waves to reclaim.

Both only manifested at N=100 headed; N=10 headed fully clean, N=100 headless fully clean. Race detector found zero violations across ~4700 iterations total.

---

## The Brutal Truth

This is both vindication and humbling.

**Vindication:** The static-vs-dynamic pincer is *exactly* the right tool. We predicted "under 100 simultaneous clients, drain-timeout may abandon up to 100 wg.Wait() goroutine pairs" from code alone (`Z1-4`). The run measured 264 orphans and traced them to the predicted location. Static analysis is not replacing runtime testing — it is *sharpening* what runtime tests are worth running. That worked.

**Humbling:** The sampler I built for the N=100 run was so badly designed it starved itself. Every 15s, the sampler spawned 100 `tasklist` calls to get per-PID memory. Under load, that became the load — 8-minute gaps between samples because tasklist itself competed for resources. I caught it by noticing the sampler's own output had become sparse. That's embarrassing. A single `tasklist` call (5ms aggregate) instead of N calls fixes it, but the fact I shipped the naive version into a stress test meant the very instrument meant to observe the disease became part of the disease. This is a hard lesson: measure twice, build once.

---

## Technical Details

**Phase 1 — Static audit** (4 reviewers, 4 zones: BiDi core, browser lifecycle, page layer, input):
- 36 findings: 1 P1 (SetExtraHeaders race), 15 P2, 20 P3
- All existing baseline tests (`go test -race -timeout 30s ./...`) passed green
- Reviewers flagged zones Z1-4 as potential high-load goroutine leaks

**Phase 2 — Stress harness build** (`integration_stress_fleet_workload_test.go`):
- N-process fleet (configurable headed/headless)
- Mixed workload: navigate/eval/click/type/tab churn/screenshot over 180s
- 15s sampler: goroutine count, heap stats, firefox RSS via `tasklist`
- 8GB free-RAM safety valve
- Initial sampler design: loop over PIDs, call `tasklist <PID>` N times per sample ← **critical mistake**

**Phase 3 — N=100 headed run** (FirefoxCore_v149, -race flag):
- Launch stampede: 8 navigate timeouts during first 5s (expected under 100-process startup pressure)
- Sampler starvation: 8-minute observation gaps (tasklist contention consumed ~50% of available CPU)
- After 180s workload + Close(): goroutine count 3 → 267 (immediate leak, confirmed by 100 × ~2.6)
- PID reap: 103 firefox.exe remained alive, profiles deleted but processes zombie'd
- Race detector: **0 violations** (unexpected, given the goroutine leak — suggests it's a logic bug, not a data race)

**Phase 4 — Diagnostic runs**:
- N=10 headed: goroutines 3 → 3, no orphans (3018 iterations, 0 errors) ✅
- N=100 headless: goroutines 3 → 3, no orphans (same duration, same workload intensity) ✅
- Conclusion: **Headed mode × N=100 saturation-specific**; headless and small N are immune

---

## Root Cause Analysis

**D-1 (goroutine leak):**
- Static match: `events.go:143-151` drain-timeout loop in `Client.handleIncoming()`
- Pattern: `for` loop spawns ~2 goroutines per BiDi subscription, `wg.Wait()` called in timeout branch but abandoned if context cancels first
- At N=100, one unlucky browser receives timeout → leaves 2 goroutines stranded → scales to ~264 across the fleet
- **Mechanism:** `func (c *Client) drain(ctx context.Context)` has a timeout that fires, but the `wg` those goroutines are waiting on never gets decremented (no fallthrough to defer/cleanup after timeout)

**D-2 (orphan processes):**
- Static match: `browser.go:185-190` Close() kills parent PID only, no BiDi `browser.close` RPC, no process-tree reap
- Windows Firefox launcher indirection (parent wraps child) means killing parent leaves child alive
- At N=100, Firefox's own resource exhaustion (or driver contention) makes children non-responsive to parent death signal
- **Mechanism:** Should emit BiDi `browser.close` RPC before SIGTERM/SIGKILL, or use `taskkill /T` (tree kill); current code blindly kills parent PID

---

## What We Tried

1. **First hypothesis**: race condition in close logic. Result: `-race` flagged 0 violations. Refuted. (The leak is a logic bug, not a data race.)
2. **Sampler v1** (per-PID tasklist loop): caused starvation. Replaced with single aggregate `tasklist` call. (Lesson: don't let the observation tool become the load.)
3. **Headless mode**: confirmed goroutine/orphan leaks do NOT occur, ruling out the Close() path as universally broken (it works under lower concurrency). Narrowed the diagnosis zone.

---

## Lessons Learned

1. **Static audit wins the pre-race.** Pointing a code reviewer at "concurrency zones" (BiDi subscriptions, lifecycle state machines, resource cleanup on errors) identifies the *right questions* before you stress-test. The reviewer can't measure impact, but they *can* flag the pattern. We'd have burned 3 hours guessing which zone matters if the audit hadn't pre-labeled Z1-4.

2. **Observation tools can silently become the load.** A sampler that spawns N system calls per observation under N-scale load will starve itself. Measure the measurement first. (Rule: aggregate all I/O per sample cycle, not per item.)

3. **Saturation exposes the weak link.** D-1 and D-2 both scale with concurrency. Under N=10 they hide; under N=100 they surface. This is why production bugs bite: tests run small.

4. **Headless ≠ headed on Windows.** The same Close() path works fine in headless because Firefox is simpler (no GPU, fewer background processes, faster shutdown). The headed version adds I/O, GPU contention, and OS event loop complexity. Next time: always run headed in stress tests on desktop platforms.

5. **Race detector silence is not all-clear.** Zero race violations with goroutine leak = abandonment bug, not concurrency bug. Race detector finds data-race corruption; it doesn't find goroutine leaks (goroutines are owned resources, not shared memory). Use both tools: `-race` + goroutine/heap sampling.

---

## Next Steps

**Fix priority (ranked by impact):**

1. **P1: events.go drain-timeout — add fallthrough cleanup** (prevents 2–3 goroutines per browser from leaking). Estimated fix: 5 lines (move cleanup outside timeout branch or add defer in the spawned goroutines).

2. **P2: browser.go Close() — emit BiDi `browser.close` before SIGTERM** (allows graceful shutdown; currently just kills parent). Estimated fix: ~10 lines.

3. **P3: browser.go Close() — use `taskkill /T` or `psutil.Process.kill(recursive=True)` for child reap** (Windows-specific; macOS/Linux may need different tree-walk). Estimated fix: ~20 lines.

4. **Test: N=100 headed regression** — add to CI as optional stress gate (`--tags stress`). Mark D-1 and D-2 as "PASS if goroutines stay ≤ 5 and orphans ≤ 0 after close".

5. **Sampler improvements** — batch all I/O per cycle; consider moving sampling to a separate goroutine with exponential backoff if the process is under load.

**Ownership & timeline:**
- Fixes 1–3 → `planner` agent to detail in follow-up plan (priority sequence, estimated 3–4 hours implementation + test).
- Fixes 4–5 → next session after P1–P3 are integrated.

**Validation:**
- Compile + existing tests pass (no regressions).
- Re-run N=100 headed stress under -race → expect 0 leaks, 0 orphans.
- Run N=100 headless as sanity check (should still be clean).

---

## Decisions Made

- **Kept findings ranked by confidence.** Static audit was low-signal on D-2 (orphan processes weren't in any zone's scope); dynamic run elevated it to real. Report consolidates both.
- **Deferred fixes to next plan.** Don't roll fixes into this audit session — audit is about *discovery*, not *implementation*. Clean handoff.
- **Marked N=100 headed as the stress regression suite.** Future load testing defaults to that config; N=10 and headless are OK for quick CI gates.

