# Migrating from chromekit

firefoxkit's public API is **drop-in compile** with chromekit for all symbols listed below. A
migration requires exactly three mechanical changes; the rest of your code compiles unchanged.

## Mechanical replacements

```diff
-import "github.com/multiprofilevn/chromekit"
+import "github.com/multiprofilevn/firefoxkit"

-b, _ := chromekit.Connect(9222)
+b, _ := firefoxkit.Connect(9333)          // Firefox default BiDi port

-chromekit.WithInputBackend(chromekit.BackendCDP)
+firefoxkit.WithInputBackend(firefoxkit.BackendBiDi)
```

| chromekit | firefoxkit | Note |
|---|---|---|
| `chromekit.*` | `firefoxkit.*` | package prefix only |
| `BackendCDP` | `BackendBiDi` | BiDi replaces CDP for input dispatch |
| `Launch(chromePath, ...)` | `Launch(firefoxPath, ...)` | path to `firefox.exe` / `firefox` binary |
| `WithFlag(key, value any)` | `WithFlag(key, value string)` | string-typed; Firefox CLI args are always strings |

## Context removal

All methods are **context-free** at the call site. firefoxkit mirrors chromekit's ctx-free surface:

- `Page.Navigate(url, timeout)` — timeout via `time.Duration`, not `ctx`
- `Page.WaitForSelector / WaitForReady / WaitForFunction / WaitForChangeURL` — same pattern
- `Mouse / Keyboard / Cookies / Storage` methods — no ctx parameter
- `Browser.Focus(ctx)` — retains ctx to match chromekit's signature; win32 calls are synchronous

## What matches exactly (drop-in compile)

| Symbol group | Status |
|---|---|
| `Connect / Launch` signatures | ✅ match |
| `Browser.{NewPage, ClosePage, SwitchPageByIndex, SwitchPageByURL, SwitchPageByOrigin}` | ✅ match |
| `Browser.{Focus, GoBack, ContentOffset, SetHWND, PID, ClosePagesNotMatching}` | ✅ match |
| `Browser.{GrantPermissions, ResetPermissions, Close, Current, Pages, InputBackend, Done}` | ✅ match |
| `Page.{Navigate, Reload, GoBack, GoForward}` | ✅ match |
| `Page.{WaitForSelector, WaitForReady, WaitForFunction, WaitForChangeURL}` | ✅ match |
| `Page.{Evaluate, EvalSelector, GetText, GetAttribute, ScrollIntoView}` | ✅ match |
| `Page.{Click, ClickElement, Type, SetInputFiles}` | ✅ match |
| `Page.{GetTitle, GetURL, GetOrigin, BringToFront, Index}` | ✅ match |
| `Page.{Screenshot, ScreenshotClip, ScreenshotElement, FullScreenshot}` | ✅ match |
| `Page.{SetExtraHeaders, SetSelectValue}` | ✅ match |
| `Page.{Cookies, LocalStorage, SessionStorage, Mouse, Keyboard, Downloads}` | ✅ match |
| `Page.{OnConsole, OnDialog}` | ✅ match |
| `MouseAPI.{Click, ClickAt, DoubleClick, RightClick, Hover, MoveTo, Scroll, Drag, DragAndDrop, Tap, FocusElement}` | ✅ match |
| `KeyboardAPI.{Press, Down, Up, Type, TypeFast, TypeHuman, TypeSelector, ClearInput}` | ✅ match |
| `CookiesAPI.{Get, Set, Delete, Clear}` | ✅ match |
| `Storage.{Get, Set, Remove, Clear}` | ✅ match |
| `Permission` constants (all 10) | ✅ match |
| `ConsoleEvent{Level, Text}` · `ConsoleLevel` constants | ✅ match |
| `DialogEvent` · `DialogAction` · `DialogType` constants | ✅ match |
| `Cookie` struct fields | ✅ match |
| `TypeSelectorConfig.{Strict, MaxAttempts, Human}` | ✅ match |
| `CursorProfile` constants | ✅ match |

## Documented exceptions (not drop-in)

These four differ by design; callers that use them need a small adjustment:

### 1. `QuerySelector` / `QuerySelectorAll` return type

| chromekit | firefoxkit |
|---|---|
| `*cdp.Node` / `[]*cdp.Node` | `*ElementHandle` / `[]*ElementHandle` |

`cdp.Node` is a chromedp type unavailable in firefoxkit. `*ElementHandle` carries the same DOM
reference semantics: use `h.EvaluateOn(ctx, fn, &out)` to read properties. Always `defer h.Dispose()`.

### 2. `BoundingBox(node *cdp.Node)` vs `BoundingBox(sel string)`

chromekit takes a pre-queried `*cdp.Node`; firefoxkit takes a selector string directly (BiDi
does not have a persistent node reference for this call path). Callers that stored a node from
`QuerySelector` must pass the original selector string instead.

chromekit `BoundingBox` also exposes `Left / Right / Top / Bottom` convenience fields; firefoxkit
`BoundingBox` exposes `X / Y / Width / Height` plus `Center()`. Adjust struct-literal access if
your code reads those fields directly.

### 3. `GrantPermissions` is a runtime no-op

Firefox's WebDriver BiDi protocol has no per-origin permission grant API. `GrantPermissions` is
present for API parity and always returns nil, but emits a warning — no grant is applied.

Use launch-time options instead:

```go
firefoxkit.WithPermissionsDefault(firefoxkit.PermGeo, true)   // blanket-allow geolocation
firefoxkit.WithPref("permissions.default.camera", 1)          // raw escape hatch
```

### 4. `KeyboardAPI.Down` / `Up` on native backend

On `BackendBiDi` (default), `Down` and `Up` work correctly (hold/release a key independently).
On `BackendNative` (winapi), they map to `SendInput` which does not model held state — use
`KeyboardAPI.Press` for native-backend key sequences. This matches chromekit's native-backend
limitation.

## Things only in firefoxkit (no chromekit equivalent)

| Symbol | Purpose |
|---|---|
| `WithPermissionsDefault(perm, allow)` | Blanket permission grant at launch (replaces runtime `GrantPermissions`) |
| `WithDownloadDir(dir)` + `WithAcceptDownloads(bool)` | Launch-time download config (chromekit used CDP `Browser.setDownloadBehavior`) |
| `WithPref(key, value any)` | Raw Firefox `user.js` preference |
| `WithLaunchTimeout(d)` | Override the 30 s BiDi session bootstrap budget |
| `Spawn(path, opts)` / `SpawnedFirefox` | Start Firefox without BiDi (anti-detect profiles) |
| `BackendBiDi` enum (replaces `BackendCDP`) | BiDi input dispatch |
| `Page.TitleLive(ctx)` / `Page.URLLive(ctx)` | Live-read title/URL bypassing cache |
| `Page.CheckBox(sel, bool)` | Set `el.checked` directly |
| `Page.SetValue(sel, value)` | Set `el.value` + dispatch input/change events |
| `Page.ClickAt(x, y)` / `Page.HoverAt(x, y)` | Page-level coordinate shortcuts |
| `Page.WaitForSelectorGone(sel, timeout)` | Wait for selector to disappear |
| `MouseAPI.MiddleClick(x, y)` | Middle-button click |
| `Storage.Keys()` | List all keys in localStorage/sessionStorage |
| `ConsoleEvent.Method` / `ConsoleEvent.Timestamp` | Extended console metadata |
| `ElementHandle` + `Dispose` / `EvaluateOn` | DOM node reference from QuerySelector |
| `BoundingBox.Center()` | Midpoint of a bounding rect |

## Behavior notes

- **`Connect`** requires Firefox 129+ launched with `--remote-debugging-port=N`.
- **`Downloads().SetPath(dir)`** is only effective when set BEFORE Launch; Firefox reads prefs at
  startup. Use `WithDownloadDir(...)` + `WithAcceptDownloads(true)` at Launch.
- **`SetExtraHeaders`** uses `network.addIntercept` (one extra round-trip per intercepted request)
  instead of CDP `Network.setExtraHTTPHeaders`.
- **`TypeFast`** bulk-inserts via one `input.performActions` call; chromekit's CDP backend used
  `Input.insertText`. Behavior at the application level is identical.
- **`FullScreenshot(quality)`** with quality < 100 re-encodes as JPEG client-side (BiDi has no
  native JPEG capture). quality ≥ 100 returns PNG.

## Example drop-in pattern

```go
// chromekit caller:
b, _ := chromekit.Launch(`C:\Program Files\Google\Chrome\Application\chrome.exe`)
defer b.Close()
b.GrantPermissions("https://example.com", chromekit.PermissionGeolocation)

// firefoxkit equivalent (3 changes highlighted):
b, _ := firefoxkit.Launch(                           // ← 1: import + package
    `C:\Program Files\Mozilla Firefox\firefox.exe`,  // ← 2: Firefox path
    firefoxkit.WithInputBackend(firefoxkit.BackendBiDi), // ← 3: BiDi backend
    firefoxkit.WithPermissionsDefault(firefoxkit.PermGeo, true), // ← runtime grant → launch-time
)
defer b.Close()
// b.GrantPermissions(...) compiles but is a documented no-op; use WithPermissionsDefault above

p := b.Current()
p.Navigate("https://example.com", 30*time.Second)  // identical
p.Click("#submit")                                  // identical
```
