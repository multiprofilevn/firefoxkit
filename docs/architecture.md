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

## Connect flow

Same as Launch but skips steps 1-5: assumes Firefox already listening on
the user-supplied port. `Close()` does NOT terminate the Firefox process
(b.closeProc == false).
