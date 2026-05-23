# Migrating from chromekit

firefoxkit mirrors chromekit's API surface ~85%. Most chromekit code drops in with two changes: import path + a couple of renamed enums.

## Mechanical replacements

```diff
-import "github.com/tuwibu/chromekit"
+import "github.com/tuwibu/firefoxkit"

-b, _ := chromekit.Connect(9222)
+b, _ := firefoxkit.Connect(9333)

-chromekit.WithInputBackend(chromekit.BackendCDP)
+firefoxkit.WithInputBackend(firefoxkit.BackendBiDi)
```

| chromekit | firefoxkit |
|---|---|
| `chromekit.*` | `firefoxkit.*` |
| `BackendCDP` | `BackendBiDi` |
| `WithFlag(key, value any)` | `WithFlag(key, value string)` — string-typed for FF arg style |
| `Launch(chromePath, ...)` | `Launch(firefoxPath, ...)` |

## Method signature deltas

Most methods are signature-identical. The ones that differ:

| chromekit | firefoxkit | Reason |
|---|---|---|
| `Mouse().ClickAt(x, y)` | `Mouse().ClickAt(ctx, x, y)` | All BiDi calls take ctx |
| `Mouse().Scroll(deltaX, deltaY)` | `Mouse().Scroll(ctx, x, y, deltaX, deltaY)` | BiDi wheel needs origin |
| `Page.Cookies()` | `Page.Cookies()` | Same API; `Cookie.Expires` now `time.Time` instead of int64 |
| `Page.Screenshot()` | `Page.Screenshot(ctx)` | BiDi async |

## Behavior deltas

- **`GrantPermissions` no longer works at runtime.** Use `WithPermissionsDefault(perm, allow)` at launch. See [permissions-gap.md](permissions-gap.md).
- **`Connect` requires Firefox 129+** launched with `--remote-debugging-port=N`. Pre-129 uses CDP which Mozilla removed.
- **`Downloads().SetPath(dir)`** only takes effect when set BEFORE Launch (writes `browser.download.dir` into user.js). Setting after Launch is a no-op — Firefox reads prefs at startup. Use `WithDownloadDir(...)` + `WithAcceptDownloads(true)` at Launch instead.
- **`SetExtraHeaders`** uses `network.addIntercept` instead of CDP `Network.setExtraHTTPHeaders`. Slightly higher latency per request (one extra round-trip per intercepted request).
- **TypeFast** bulk-inserts via one `input.performActions` call (chromekit's CDP backend used `Input.insertText`).

## Things that work the same

- `Navigate / Reload / GoBack / GoForward / WaitFor*`
- `Click(sel) / Hover(sel) / Type(text)`
- `Evaluate / QuerySelector / BoundingBox / GetText / GetAttribute / ScrollIntoView`
- `Keyboard().TypeHuman / TypeSelector` — same timing & typo algorithm
- `Storage().Local() / Session()` — Get/Set/Remove/Clear/Keys
- Native input backend — Windows-only winapi `SendInput`, **same code as chromekit** with only the window class (`MozillaWindowClass`) and default content offset adjusted

## Things missing in firefoxkit (vs chromekit)

- Runtime per-origin `GrantPermissions` — see permissions-gap.md
- BiDi-level fine-grained network interception with body rewrite (basic header-mutation works)
- `FullScreenshot(quality)` with JPEG quality < 100 re-encodes client-side (no native JPEG support in BiDi)

## Things in firefoxkit that chromekit doesn't have

- `WithPermissionsDefault(perm, allow)` — Firefox-specific blanket grants
- `WithDownloadDir(dir)` + `WithAcceptDownloads(true)` — launch-time download config (chromekit relied on CDP `Browser.setDownloadBehavior`)
- `Page.SetValue(sel, val)` / `Page.CheckBox(sel, bool)` — JS-level form helpers added because file upload via DataTransfer needs them
