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
  Works headless, works with hidden tabs.
- `BackendNative` — Windows-only. Drives the OS cursor + keyboard via
  `winapi.SendInput`. Bypasses `isTrusted=false` fingerprinting at the cost
  of requiring a visible focused window.

Selection happens per-Page via `Browser.cfg.inputBackend`. The same Backend
interface lets `Keyboard` (TypeHuman algorithm) and `Cursor` (Bezier path)
layer human-like behavior on top.

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
  modifier keys.
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

`integration_stress_test.go` (build tag `stress`) verifies the leak fixes
against a real Firefox 129+. Not part of the default `go test`. Run manually:

```
FIREFOXKIT_FIREFOX_PATH=/path/to/firefox go test -tags stress -race -run Stress -v -timeout 20m
```

Covers: goroutine leak on Launch/Close churn, Firefox-side RSS after 1000
`QuerySelectorAll` calls, tab-churn page pruning, console-flood goroutine
bound, and temp-profile-dir cleanup. Each test skips when
`FIREFOXKIT_FIREFOX_PATH` is unset.
