# Architecture

firefoxkit splits cleanly into three layers.

```
+----------------------------------------+
|  Public package: firefoxkit/           |
|  Browser, Page, Frame, ElementHandle,  |
|  Mouse, Keyboard, Cookies, Storage,    |
|  Downloads, screenshot, listeners...   |
+--------------------+-------------------+
                     |
            +--------v---------+    +--------------------+
            |  input/          |    |  internal/bidi/    |
            |  Backend iface,  |    |  WebDriver BiDi    |
            |  BackendBiDi,    +--->+  client (WS+JSON), |
            |  BackendNative   |    |  command/event     |
            |  (winapi),       |    |  router, typed     |
            |  TypeHuman algo  |    |  wrappers per      |
            +------------------+    |  domain            |
                                    +--------------------+
```

## Public package (`firefoxkit/`)

User-facing types. Methods translate ergonomic Go calls into BiDi commands
via `internal/bidi.Client`. Per-page state (header injector, downloads
channel, listeners) is lazy-initialized via `sync.Once` so unused features
cost nothing.

## Input package (`input/`)

Backend interface + two implementations:

- `BackendBiDi` — issues `input.performActions` on the BiDi client. Default.
  Works headless, works with hidden tabs. Multiple pages/browsers can gesture
  in parallel (Firefox parallelizes across browsing contexts).
- `BackendNative` — Windows-only. Drives the OS cursor + keyboard via
  `winapi.SendInput`. Bypasses `isTrusted=false` fingerprinting at the cost
  of requiring a visible focused window. **Atomic per-gesture:** all gestures
  (move→down→up, click, drag, tap, scroll) across ALL native browsers in the
  process are serialized by a package-level gesture lock. With N concurrent
  native browsers (production max 9), native input serializes per gesture
  (single OS cursor is the bottleneck). This is by design — there is only one
  desktop pointer; gestures must not interleave.

Selection happens per-Page via `Browser.cfg.inputBackend`. The same Backend
interface lets `Keyboard` (TypeHuman algorithm) and `Cursor` (Bezier path)
layer human-like behavior on top. BiDi backend is unaffected and remains
parallel.

## BiDi client (`internal/bidi/`)

WebSocket client implementing the W3C BiDi protocol. Components:

- `client.go` — Send/await pattern with id correlation, reader goroutine,
  Close drains pending. EventBus for event subscription.
- `transport.go` — WebSocket wrapper (`coder/websocket`).
- One file per BiDi domain: `browsingcontext.go`, `script.go`, `input.go`,
  `storage.go`, `network.go`, `log.go`. Each exposes typed Go wrappers for
  the subset of commands firefoxkit consumes.
- `events.go` — subscription registry with panic-safe dispatch.

Subset-only philosophy: a BiDi command is only added when a public API
needs it. The full BiDi spec is large; we don't pay maintenance cost on
commands no one calls.

## Launch flow

1. `Launch(path, opts...)` builds config from opts.
2. Picks free TCP port. Creates temp profile dir (or uses `WithUserDataDir`).
3. Writes `user.js` (BiDi-enabling prefs + proxy + user-supplied prefs).
4. Spawns Firefox with `--remote-debugging-port=N -profile <dir> -no-remote`.
5. Discovers BiDi WS URL: races stderr regex scan and TCP probe.
6. Opens `bidi.Client`, calls `session.new` with capabilities.
7. `getTree` populates initial Page list. Returns Browser.

## Public API contract: context propagation

**Every public method that issues a BiDi RPC must accept `ctx context.Context` as its first argument. Never use `context.Background()` internally for caller-initiated work.**

Why: the library lives inside larger pipelines (HTTP handlers, background workers, test runners). Callers expect cancellation to propagate end-to-end. If `Navigate(url, timeout)` synthesizes its own background ctx, an HTTP handler whose request was canceled keeps a stale Firefox tab loading for 30s after the client gave up — wasting BiDi quota and leaking goroutines.

Rules:
- **Direct RPC** (Evaluate, Screenshot, Cookies, BoundingBox, Click, Type, ...): take ctx explicitly.
- **Polling helpers** (WaitForReady, WaitForSelector, ...): take ctx, derive per-attempt sub-contexts from it, propagate cancellation both inward and outward.
- **Event subscriptions** (OnDialog, OnConsole): take ctx for the *subscription call*, but the handler invocations happen on the event-bus goroutine with their own ctx (typically a fresh `context.Background` since the event isn't bound to a caller).
- **Lifecycle** (Close, ClosePage): may use `context.Background()` for shutdown — those are not caller-driven, they're cleanup.

What violates the rule: `func (p *Page) Navigate(url string, timeout time.Duration) error` — no ctx, synthesizes a background one internally. Fix: change to `func (p *Page) Navigate(ctx context.Context, url string) error` and let caller wrap with `context.WithTimeout` upstream.

## Connect flow

Same as Launch but skips steps 1-5: assumes Firefox already listening on
the user-supplied port. `Close()` does NOT terminate the Firefox process
(b.closeProc == false).

## Close flow (Launch path)

Shutdown is graceful but bounded. Three phases execute in order:

1. **Graceful quit (~2s budget):** BiDi `browser.close` RPC is best-effort,
   issued on a separate goroutine because WebSocket writes do not honor caller
   ctx. Firefox uses this to flush its profile and tear down its own content
   processes. Best-effort only — a hung peer does not hold Close hostage.
2. **Cleanup sweep & drainer join (~2s budget):** per-page cleanup RPCs and
   the drainer goroutines are signaled and waited. Runs BEFORE the BiDi client
   tears down so its RemoveIntercept/Unsubscribe RPCs still reach Firefox.
   Against a quitting peer these RPCs often fail; they are best-effort and
   logged.
3. **Process tree reap (~5s timeout):** The Firefox process tree is killed via
   Windows Job Object (created at spawn with KILL_ON_JOB_CLOSE flag; takes the
   whole tree when the handle closes) or fallback `taskkill /T /F`. Close
   blocks until the process actually exits (verified by observing the cmd.Wait
   owner's done signal) — success is not just a kill request, it is proof of
   exit. If the tree survives the kill, Close returns `"firefox pid=N survived kill"`.

Worst case is roughly 9-11s (one 2s budget for graceful + one 2s budget for
sweep + one 5s timeout for reap). Typical healthy shutdown is milliseconds
+ process exit latency.

On the Connect path (step 0 only), Close does NOT send `browser.close` or
kill anything, it just disconnects the BiDi client and lets Firefox keep
running.

## Event dispatch (bounded per-subscription workers)

`internal/bidi` delivers events off the reader goroutine: each `Subscribe`
owns a bounded queue (cap 256) drained by one worker. Consequences:

- Handlers **may** call back into firefoxkit RPCs — they no longer run on the
  reader, so a handler that issues `Send` can't deadlock the client.
- A slow handler only delays (or, past the cap, drops with a throttled warn)
  events of **its own** subscription. Other subscriptions are unaffected.
- Ordering is preserved per subscription (single worker), not across
  subscriptions.
- `Close()` blocks until the reader has exited and every worker has drained,
  so no handler runs after `Close()` returns.
- **Avoid calling `Browser.Close()`/`Page.Close()` from inside an event
  handler.** A handler runs on its subscription's worker, and `Close` waits
  for workers to drain — so a handler that closes the client waits on itself.
  This is now bounded (Close proceeds after a 5s drain timeout with a warn log
  instead of hanging), but the clean pattern is `go browser.Close()`.

## Long-running / leak safety

Fixes that keep a long-lived automation process flat (plan 260719-1620):

- `browsingContext.contextDestroyed` is subscribed once per Browser: tabs
  Firefox closes itself (self-closing popups, `window.close()`) are pruned
  from `b.pages` and cleaned up.
- `Page.cleanup` is idempotent and also `session.unsubscribe`s the per-context
  events it registered, and issues `input.releaseActions` to clear any held
  modifier keys. Local teardown always runs; under drainer saturation (>512
  pending cleanups) the RPC half is dropped (counter logged via Close) but
  page tracking stays correct.
- `QuerySelectorAll`/`QuerySelector` disown the ownership=root handles they
  create so Firefox doesn't pin result objects forever.
- Temp profile dirs are removed by `Browser.Close` (not just on the Launch
  error path).
- Mouse paths on the BiDi backend are batched into few `input.performActions`
  calls (`PathBatcher.MousePath`) instead of one RPC per point, avoiding the
  WebSocket congestion that stalls the cursor under fleet load.

## Known behavior: ctx-cancel during a WebSocket write

`coder/websocket` closes the **whole** connection if a `conn.Write` is
interrupted by ctx cancellation. So a `Send` with a very short deadline
carrying a large payload can tear down the session on a loaded machine. Give
large-payload calls a generous timeout. Documented, not worked around (see
`internal/bidi/transport.go`).

## Stress / leak-test suite

Build tag `stress` — not part of default `go test`. Several tests verify the
leak fixes against a real Firefox 129+ on a dev machine. Run manually:

```bash
# Core leak tests (20 min)
FIREFOXKIT_FIREFOX_PATH=/path/to/firefox go test -tags stress -race -run Stress -v -timeout 20m

# Mixed-workload fleet (N browsers, realistic load)
FIREFOXKIT_FIREFOX_PATH=/path/to/firefox \
  FIREFOXKIT_FLEET_N=100 FIREFOXKIT_HEADED=1 FIREFOXKIT_WORKLOAD_SECONDS=300 \
  go test -tags stress -race -run TestStress_FleetWorkload -v -timeout 60m
```

**Core tests** (`integration_stress_test.go`): goroutine leak on Launch/Close
churn, Firefox-side RSS after 1000 `QuerySelectorAll` calls, tab-churn page
pruning, console-flood goroutine bound, temp-profile-dir cleanup.

**Fleet workload** (`integration_stress_fleet_workload_test.go`): N separate
Firefox processes each running a mixed automation loop (navigate → evaluate →
click → type → tab churn → screenshot) while a sampler tracks goroutines, Go
heap, and total Firefox RSS over time. Knobs: `FIREFOXKIT_FLEET_N` (default
25), `FIREFOXKIT_HEADED` (default headless), `FIREFOXKIT_WORKLOAD_SECONDS`
(default 180), `FIREFOXKIT_NAV_TIMEOUT_SECONDS` (bounds single Navigate, default
derived from fleet size). Safety valve: aborts if available physical memory
drops below 8 GB.

Each test skips when `FIREFOXKIT_FIREFOX_PATH` is unset.
