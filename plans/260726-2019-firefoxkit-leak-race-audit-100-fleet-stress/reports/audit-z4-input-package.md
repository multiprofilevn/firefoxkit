# Zone Z4 — input package: static leak/race audit

Scope: `input/*.go` (14 files) + wiring `page_input.go`, `page_input_backend.go`, `page_mouse_api.go`,
`page_keyboard_api.go`, `page_type_selector.go`, `browser_focus_windows.go`. Read-only, no code changed.

## Findings

| ID | file:line | type | sev | evidence | fix (1 line) | conf |
|----|-----------|------|-----|----------|--------------|------|
| Z4-1 | `input/backend_native_windows.go:157,163-176` | global-state-race | P2 | `nativeInputMu` is held per **syscall**, not per gesture. `MouseDown` = `MouseMove` (lock/unlock at :157) **then** `sendMouse` (lock/unlock at :313). Two Browsers on `BackendNative` interleave: B's `SetCursorPos` lands between A's move and A's button-down → A's click fires at B's coordinate, possibly in B's window. `SetCursorPos`/`SendInput` are desktop-global; per-`Cursor` `c.mu` (`cursor_engine.go:94`) cannot serialize across instances. | Add a package-level gesture lock (`Lock`/`Unlock` around the whole Down+Up / whole path), or export `LockInput()` for fleet callers | high |
| Z4-2 | `page_mouse_api.go:159-187` | race | P2 | `Drag` takes the raw backend (`:160`) and issues `MouseDown`/`MouseMove`×11/`MouseUp` **outside** the cursor engine mutex. A concurrent `MoveTo` on the same Page holds `c.mu` and emits `pointerMove` on the *same* BiDi pointer source id `"ffk-mouse"` (`backend_bidi.go:25,65`) while the drag's button is held → interleaved pointer state; drop can land at the other move's coords. | Route the drag body through the cursor engine (add `Cursor.Drag` holding `c.mu`) instead of the raw backend | high |
| Z4-3 | `page_mouse_api.go:187`, `input/touch.go:23-27` | state-leak | P2 | `Drag` ends with `b.MouseUp(ctx, …)` on the **caller** ctx; `Tap` likewise after an uninterruptible `time.Sleep`. If ctx died mid-gesture the release RPC fails and the button stays pressed in Firefox. `Cursor.Click` already handles exactly this with a 2s background ctx (`cursor_engine.go:203-210`) — Drag/Tap do not. Only `p.cleanup` → `Release` (`page.go:133-140`) eventually recovers it. | Mirror the `Cursor.Click` pattern: release on `context.WithTimeout(context.Background(), 2s)` when ctx is dead | high |
| Z4-4 | `input/keyboard_human.go:16,51,61-62` | race | P2 | `HumanTypeConfig.Rand *rand.Rand` is called (`Float64`/`Intn`) with **no** synchronization. `*rand.Rand` is not goroutine-safe. The config is passed by value but the pointer is shared, so one `TypeSelectorConfig{Human:{Rand:…}}` reused across pages/goroutines (`page_type_selector.go:20,49`) is a data race — and that is the pattern the tests model (`keyboard_human_test.go:84,103`). No doc warning. | Doc "Rand must not be shared across goroutines", or wrap it in a mutex-guarded adapter inside `TypeHuman` | high |
| Z4-5 | `input/cursor_engine.go:57,149`; `input/cursor_remote.go:53-60` | unbounded-growth | P2 | Remote path is fetched on **every** ProfileHuman `MoveTo`. The per-Cursor `&http.Client{Timeout:…}` has a nil Transport → shared `http.DefaultTransport`, `MaxIdleConnsPerHost = 2`. At 100 browsers moving concurrently ≈98 of every 100 connections are closed instead of pooled; the non-2xx path (`cursor_remote.go:58`) also returns without draining the body, killing keep-alive. On Windows that is TIME_WAIT / ephemeral-port pressure. | Share one `*http.Client` with a tuned `Transport{MaxIdleConnsPerHost: N}` across all Cursors; drain body before close | med |
| Z4-6 | `input/cursor_engine.go:95-96,141-160` | deadlock-adjacent (lock-under-IO) | P2 | `MoveTo` holds `c.mu` for its entire body, and `buildPath` performs the blocking HTTP POST inside it. With the endpoint down every move blocks ~1200ms (`remotePathTimeout`) **under the lock**, and there is no failure memoization/circuit-breaker — the cost is paid per move, forever. `Position()`, `SetTuning()`, `Scroll()` (`:63,:18,:215`) all queue behind it. | Fetch the remote path before taking `c.mu`, and short-circuit remote for N seconds after a failure | high |
| Z4-7 | `input/cursor_path.go:44-60` ← `cursor_engine.go:161` | unbounded-growth | P3 | `humanStepCount` clamps `maxSteps` only from below (`<=0 → 80`); `CursorTuning.MaxSteps` (new, `options.go:123`) has **no upper bound**. `MaxSteps = 1_000_000` allocates a 1M-`Point` + 1M-`PathPoint` slice per move and issues ~5000 chunked `performActions` (`backend_bidi_mousepath.go:27,47`). Remote paths *are* capped (`cursor_remote.go:75`, 2000) — inconsistent. | Clamp `MaxSteps` to `maxRemotePathPoints` (2000) in `humanStepCount` | med |
| Z4-8 | `browser.go:119` vs `browser_focus_windows.go:76` | race | P3 | `SetHWND` writes `b.hwndOverride` with no lock/atomic; `HWND()` reads it from any goroutine — reached lazily from `p.nativeWindow()` → `ContentOffset()` during first-use backend construction (`page_input_backend.go:27,43`). Doc says "Connect path" but nothing enforces before-use ordering. | Make `hwndOverride` an `atomic.Uintptr` | high |
| Z4-9 | `input/backend_native_windows.go:206,215,254,259,263,268`; `keyboard.go:180,183`; `touch.go:26` | (cancellation) | P3 | Plain `time.Sleep` in the native key/type/combo paths and `Keyboard.sleep`. A cancelled `InsertText` still burns up to ~150ms per char before the next `ctx.Err()` check (`:200`), `KeyCombo` up to ~140ms after cancel, and `Tap`'s hold is uninterruptible. `sleepCtx` (`keyboard_human.go:106-118`) already exists and is correct. | Replace those `time.Sleep` calls with the existing `sleepCtx(ctx, d)` | high |
| Z4-10 | `input/backend_native_windows_offset.go:45-47,66-67` | memory-leak (minor) | P3 | `syscall.NewLazyDLL("user32.dll")` + `NewProc` are re-created **per call** inside `MeasureContentOffset`/`MeasureDPIScale`; each first use does a `LoadLibrary` with no matching `FreeLibrary`, so the module refcount grows once per page/backend init across 100-browser churn. The same file's sibling (`backend_native_windows.go:74-79`) already hoists these to package vars. | Hoist `user32`/`getClientRect`/`clientToScreen`/`getDpiForWindow` to package-level vars | med |
| Z4-11 | `input/keyboard.go:127,176-183` | race | P3 | `Keyboard.MinDelay`/`MaxDelay` are **exported mutable fields** on an object cached per page (`page_keyboard_api.go:10`), read by `sleep()` with no lock. Any caller tuning `p.Keyboard()` timing while another goroutine is inside `Type` races. | Unexport + add `SetDelays()` under a mutex (mirrors `Cursor.SetTuning`) | med |

**No P1.** Nothing in Z4 leaks a goroutine or grows memory without bound on its own.

## Clean areas (verified)

- **No `go` statements anywhere in `input/`** → zero goroutine-leak surface in the zone.
- `sleepCtx` (`keyboard_human.go:106-118`): `defer t.Stop()` + select on `ctx.Done()` — no timer leak.
- `cursor_remote.go`: ctx-bound request, `defer resp.Body.Close()` on every post-error path, status check, NaN/Inf rejection, `maxRemotePathPoints` + `maxRemoteDelayMs` clamps, endpoint failure always falls back to local bezier.
- `Cursor` state: `x/y/tuning/mousePathURL` all read+written under `c.mu`; `Position`/`SetPosition`/`SetTuning`/`Scroll` all lock correctly.
- `Cursor.Click` (`cursor_engine.go:203-210`): releases the button on a background ctx when the caller ctx dies — the correct pattern (Z4-3 is its absence elsewhere).
- `backendCache` (`page_input_backend.go:22-35` / `page.go:133-140`): `sync.Once` write + `atomic.Bool` ready flag gives a correct happens-before for the cleanup `Release` read; `Release` **is** wired (not dead code).
- `MouseAPI.lastX/lastY` guarded by `posMu` (`page_mouse_api.go:36-40,139-142`).
- No `rand.New(rand.NewSource(...))` in non-test code — all other RNG use is the global `math/rand` (locked source, goroutine-safe).
- `BackendBiDi` is stateless per call; `MousePath` chunks at 200 actions and re-checks ctx between chunks.
- `findFirefoxHWND` (`browser_focus_windows.go:92-102`): `syscall.NewCallback` allocated exactly once + `findHwndMu` around the package-var handoff — no callback-slab exhaustion, no per-lookup leak.
- `cssToScreen`/`screenToCSS` are pure; `humanDelayFunc` bound-normalization cannot produce `rand.Intn(0)`.

## Unresolved questions

1. Is `BackendNative` ever intended to run with >1 concurrent Browser? If yes, Z4-1 needs a fleet-wide gesture lock, not a per-syscall one.
2. Is `WithMousePathURL` used in the 100-fleet stress config? Z4-5/Z4-6 only bite when it is set.

**Status:** DONE
Audited all 14 `input/` files plus the 6 wiring files: 11 findings (6× P2, 5× P3, no P1), the highest-value ones being non-atomic native gestures (Z4-1), `Drag` bypassing the cursor mutex (Z4-2), and remote-path HTTP performed under `c.mu` with default-transport connection churn (Z4-5/Z4-6). The zone has **no goroutine leaks** — there is not a single `go` statement in the package.
