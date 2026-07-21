## Developer phase report

### Phase
- Plan: plans/260721-1950-firefoxkit-load-hardening
- Phase: phase-05-stress-harness-empirical
- Status: completed

### Files
**Backend**
- Created: `integration_stress_move_test.go` (`//go:build stress`, ~290 LOC) — 4 tests + latency/stats helpers, reuses `stressFirefox`/`settleGoroutines`/`rssKB` from `integration_stress_test.go`. No production code touched.
- Modified: `plans/reports/brainstorm-260721-1950-firefoxkit-load-audit.md` — appended "Kết quả thực nghiệm" section resolving P-open.

### Contract
N/A (test-only harness, no API surface).

### Tasks
- [x] Read phase spec + existing stress harness + Mouse/Browser/Options/cursor_path source
- [x] Write `TestStress_MoveLatency_ByProfile`, `TestStress_MoveLatency_SamePage_KGoroutines`, `TestStress_MoveLatency_KPages_Concurrent`, `TestStress_ChurnGrowth_NoLeak`
- [x] `go vet -buildvcs=false -tags stress ./...` clean
- [x] Skip verification (env unset) — all 4 SKIP cleanly, 0.77s
- [x] Real run against Firefox 149 (`FirefoxCore_v149`) — all 4 PASS, 107.3s total
- [x] Fed results back into audit report

### Verified
- Compile (`go vet -tags stress`): pass
- Skip (env unset, `-run 'TestStress_Move|TestStress_ChurnGrowth'`): pass — 4/4 SKIP
- Real Firefox run (`-run 'TestStress_MoveLatency|TestStress_ChurnGrowth' -v -timeout 20m`): pass — 4/4 PASS

### Real measured numbers (Firefox 149, Windows, headless BiDi, K=20 representative of 200-fleet)

**MoveTo latency by profile** (1 page, sequential, n=30, warmup=5):

| Profile | p50 | p95 | max |
|---|---|---|---|
| Human | 621.9ms | 685.5ms | 726.4ms |
| Fast | 218.1ms | 220.3ms | 223.6ms |
| Machine | 16.6ms | 27.7ms | 58.6ms |

**Same-page K=20 goroutines** (P2 mutex serialization): single-move avg=230.5ms, total=3.35s, **ratio=14.5x** (expect ~20x) — confirms serialization, not parallel.

**K=20 pages concurrent (serialization probe — the open question):** single-move avg=1.64s, total=1.88s, **ratio=1.15x**.
**VERDICT: PARALLEL (ratio≈1)** — Firefox does NOT serialize `input.performActions` across browsingContexts. Architecture implication: scale via more pages/browser (1 goroutine/1 page), not more browsers/machine, for input throughput; per-page mutex (P2) and CPU/RTT are the real bottlenecks, not the actor.

**Churn growth** (T=200 rounds, `WithCursorTuning{MaxSteps:12}`): goroutines flat 10→10 (delta=0), `len(Pages())` back to baseline=1 every round, RSS 6796KB→6740KB (no growth). Confirms P1/F1(timeout)/L1(prune) hold under sustained churn.

K=20/T=200 explicitly representative of the 200-process fleet — not literal 200, dev machine cannot host 200 concurrent Firefox instances.

### Issues
- README "Cursor profile / load" table states Fast ≈ 11–88ms; measured Fast p50 on BiDi backend = 218ms — real number is ~2.5-10x the documented range. Table's own header only claims accuracy for the profile ordering (Machine < Fast < Human), which held, but the absolute Fast number is stale/optimistic. Per top-level task scope ("ADD a stress test file only — do NOT modify production code"), I did NOT edit README.md — flagging here per phase step 3's own instruction to update it if numbers diverge. Recommend a follow-up doc-only edit to README's latency table with the measured range.
- Same-page K-goroutines ratio (14.5x vs 20x) is looser than a naive "fully serialized" expectation — some overlap likely from BiDi RPC pipelining or scheduling jitter; not treated as a bug, matches "loose, relative" threshold guidance in the spec.

### Next
- Consider a follow-up doc-only phase to update README's Fast/Machine latency numbers with measured values (BiDi backend, not "native backend only" as the option table currently implies — `WithCursorProfile` actually applies to BiDi cursorEngine too, confirmed in `page_input_backend.go:77-87`).
- Plan phase 5 marked complete via `claudekit plan check 5`.
