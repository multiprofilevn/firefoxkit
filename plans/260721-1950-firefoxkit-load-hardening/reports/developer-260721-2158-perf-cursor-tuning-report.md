## Developer phase report

### Phase
- Plan: plans/260721-1950-firefoxkit-load-hardening
- Phase: phase-04-perf-adaptive-cursor-knob
- Status: completed

### Files
- Created: `input/cursor_tuning.go` (CursorTuning struct + Cursor.SetTuning), `input/cursor_tuning_test.go` (4 tests)
- Modified: `input/cursor_path.go` (`humanStepCount` now takes `maxSteps int`), `input/cursor_engine.go` (`tuning` field on Cursor, `humanDelayFunc` helper, buildPath wiring), `options.go` (`WithCursorTuning`, `cursorTuning` field, `input` import), `page_input_backend.go` (`p.cursor.SetTuning(...)` after construction), `README.md` (Options table row + new "Cursor profile / load" section)

### Contract
No HTTP/DB contract — pure Go API addition. `WithCursorTuning(input.CursorTuning{MaxSteps, MinDelayMs, MaxDelayMs})` is additive; zero-value reproduces prior hardcoded behavior exactly (locked by `TestHumanPath_DefaultTuning_Unchanged`).

### Components added
- `input.CursorTuning` — value struct, 3 fields, zero-value = no-op
- `Cursor.SetTuning(t CursorTuning)` — additive setter, mutex-guarded, called post-construction (kept `NewCursorWithProfileAndPath` signature unchanged to avoid touching existing call sites/tests)
- `WithCursorTuning(t input.CursorTuning) Option` — firefoxkit root package option

### Tasks
- [x] Write `cursor_tuning_test.go` first — confirmed RED (compile fail: `SetTuning`/`CursorTuning` undefined)
- [x] Parameterize `humanStepCount(..., maxSteps int)` — clamps both floor (min(8,maxSteps)) and ceiling
- [x] Parameterize ProfileHuman delay range via `humanDelayFunc(tuning)` in `cursor_engine.go`
- [x] Wire tuning field into `Cursor`, `WithCursorTuning` option, `page_input_backend.go`
- [x] README "Cursor profile / load" table + Options row
- [x] Full build/vet/race pass

### Verified
- Build: pass (`go build -buildvcs=false ./...`)
- Vet: pass (`go vet -buildvcs=false ./...`)
- Race tests: pass — `go test -buildvcs=false -race -count=1 ./...` → root pkg 10.05s, `input` 3.82s, `internal/bidi` 7.42s, all `ok`
- New tests RED→GREEN: `TestHumanPath_DefaultTuning_Unchanged`, `TestHumanPath_MaxStepsRespected`, `TestHumanPath_DelayRange`, `TestTuning_ProfileFastMachine_Untouched` — all PASS
- Regression: `page_mouse_api_cursor_test.go`, `input/cursor_path_batch_test.go`, `input/cursor_remote_test.go` unaffected (still pass)
- `stress`-build-tag integration tests untouched/out of scope (require real Firefox; not part of this phase)

### Issues
None. Design followed the phase spec exactly — no deviations.

### Surface decisions (per rule)
- Kept `NewCursorWithProfileAndPath` signature unchanged; added `SetTuning` as a separate setter instead of a 4th constructor param — avoids touching every existing call site (`cursor_remote_test.go`, `cursor_path_batch_test.go`) while staying additive. Phase spec allowed either "via NewCursorWithProfileAndPath or a dedicated setter" — chose setter.
- `MaxDelayMs < MinDelayMs` (invalid config) collapses to a fixed `MinDelayMs` delay rather than erroring — matches KISS/no-panic guidance implicit in the spec; not explicitly tested since spec's 4 tests don't cover this edge, documented in the `CursorTuning` doc comment instead.
- No auto-adaptive-by-CPU logic added, per explicit YAGNI directive in spec.

### Next
Phase 5 (perf measurement) can validate the README latency table against real numbers.

**Status:** DONE
**Summary:** Added `input.CursorTuning` + `Cursor.SetTuning` + `WithCursorTuning` option to cap ProfileHuman step/delay under load; default behavior locked unchanged; README documents profile-by-concurrency guidance. TDD RED→GREEN, full build/vet/race clean.
**Concerns/Blockers:** none
