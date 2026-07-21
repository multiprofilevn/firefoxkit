# firefoxkit

Pure-Go Firefox automation library — counterpart of [chromekit](https://github.com/tuwibu/chromekit). Uses the W3C WebDriver BiDi protocol (Firefox 129+). No Node.js, no extra runtime.

## Install

```bash
go get github.com/tuwibu/firefoxkit
```

Requires Firefox **129 or newer** (BiDi protocol — CDP support was removed in FF 129).

## Quickstart — attach to running Firefox

Start Firefox with the BiDi remote-debugging port:

```
firefox.exe --remote-debugging-port=9333 -profile C:\tmp\ff-profile -no-remote
```

Then attach:

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/tuwibu/firefoxkit"
)

func main() {
    b, err := firefoxkit.Connect(9333)
    if err != nil {
        log.Fatal(err)
    }
    defer b.Close()

    p := b.Current()
    if err := p.Navigate("https://example.com", 30*time.Second); err != nil {
        log.Fatal(err)
    }

    img, _ := p.Screenshot(context.Background())
    os.WriteFile("out.png", img, 0o644)
}
```

Use `Launch` instead if you want firefoxkit to spawn Firefox itself:

```go
b, _ := firefoxkit.Launch(
    `C:\Program Files\Mozilla Firefox\firefox.exe`,
    firefoxkit.WithHeadless(false),
)
```

`Connect` reuses the user's tabs and never kills Firefox on `Close()`. `Launch` owns the process and shuts it down.

## Core API

| Layer    | Method                                        | What it does                              |
|----------|-----------------------------------------------|-------------------------------------------|
| Browser  | `Connect(port, opts ...Option)`               | attach to Firefox already running on the BiDi port |
| Browser  | `Launch(firefoxPath, opts ...Option)`         | spawn a new Firefox process |
| Browser  | `NewPage()` / `Pages()` / `Current()`         | manage tabs |
| Browser  | `SwitchPageByIndex/URL/Origin`                | switch active tab |
| Browser  | `ClosePage(id)` / `Close()`                   | clean teardown |
| Browser  | `HWND()` / `FocusWindow()`                    | Windows-only: surface window for native input |
| Page     | `Navigate(url, timeout)` / `Reload` / `GoBack` / `GoForward` | navigation |
| Page     | `WaitForReady` / `WaitForSelector` / `WaitForChangeURL` / `WaitForFunction` | sync |
| Page     | `Click(sel)` / `ClickElement(sel)` / `Type(text)` / `Hover(sel)` | high-level shortcut |
| Page     | `Evaluate(expr, out)` / `EvaluateCtx(ctx, expr, out)` / `EvalSelector(sel, fn, out)` / `EvalSelectorAll` | run JS |
| Page     | `Screenshot()` / `FullScreenshot(q)` / `ScreenshotClip(x,y,w,h)` / `ScreenshotElement(sel)` | PNG / JPEG capture |
| Page     | `QuerySelector` / `QuerySelectorAll` / `BoundingBox` / `GetText` / `GetAttribute` / `ScrollIntoView` / `Focus` | DOM |
| Page     | `SetExtraHeaders(map)` / `SetInputFiles(sel, paths...)` / `SetValue(sel, val)` / `CheckBox(sel, bool)` | request shaping / input |
| Page     | `OnDialog(handler)` / `OnConsole(handler)`    | event listeners |
| Page     | `Frames()` / `AllFrames()` / `MainFrame()` / `FrameByURL(re)` / `FrameByName(name)` | iframe access |
| Page     | `Cookies()` → `Get/Set/Delete/Clear/GetAll`   | cookie namespace |
| Page     | `Mouse()` → `MoveTo/ClickAt/Scroll/Hover/DoubleClick/RightClick/Drag/DragAndDrop` | mouse |
| Page     | `Keyboard()` → `Press/Down/Up/Type/TypeFast/TypeHuman/Combo/ClearInput` | keyboard namespace |
| Page     | `Keyboard().TypeSelector(sel, text, cfg...)`  | type via TypeHuman, verify round-trip (Strict/MaxAttempts) |
| Page     | `Storage().Local()/Session()` → `Get/Set/Remove/Clear/Keys` | web storage |
| Page     | `Downloads()` → `SetPath/Wait`                | download interception |
| Page     | `SelectByValue/Index/Text`                    | `<select>` helpers |
| Browser  | `GrantPermissions(origin, perms...)` → returns `ErrPermissionsRuntimeUnsupported` | runtime grant **not** supported — see Limitations |

## Options

| Option | Meaning |
|---|---|
| `WithLogger(l)` | inject a `Logger` (default: no-op) |
| `WithUserDataDir(dir)` | persist Firefox profile across runs |
| `WithHeadless(bool)` | toggle headless mode (default false) |
| `WithProxy(addr)` | route Firefox through `host:port` proxy |
| `WithInputBackend(t)` | `BackendBiDi` (default) or `BackendNative` |
| `WithNativeWindow(x, y, scale)` | window offset + DPI for native backend |
| `WithCursorProfile(p)` | `CursorHuman` / `CursorFast` / `CursorMachine` (applies to both BiDi and Native) |
| `WithCursorTuning(t)` | cap `ProfileHuman`'s step count / delay range (`input.CursorTuning`) — see "Cursor profile / load" below |
| `WithFlag(key, value)` | pass arbitrary Firefox CLI flag |
| `WithPref(key, value)` | raw Firefox preference (`user_pref`) |
| `WithModifyCmd(fn)` | hook `*exec.Cmd` before Firefox starts |
| `WithLaunchTimeout(d)` | override 30s default for launch + session.new |
| `WithCommandTimeout(d)` | override the 60s default backstop applied to BiDi commands whose context has no deadline (0 disables) |
| `WithPermissionsDefault(perm, allow)` | blanket allow/block at launch (no per-origin) |
| `WithDownloadDir(dir)` | save downloads here (paired with WithAcceptDownloads) |
| `WithAcceptDownloads(b)` | suppress download dialog |

Every BiDi command issued without an explicit deadline (the ctx-less API surface) falls under a **60s default command timeout** — a backstop so a lost response can't park a goroutine forever. Change it globally with `WithCommandTimeout(d)` (`0` disables). A script whose awaited promise legitimately runs longer than 60s should use `EvaluateCtx` with an explicit longer deadline, e.g. `p.EvaluateCtx(ctxWith5MinTimeout, expr, &out)` — a caller-supplied deadline always takes precedence over the backstop.

## Input backends

|                    | BiDi (default)                                   | Native                                   |
|--------------------|--------------------------------------------------|------------------------------------------|
| Mechanism          | BiDi `input.performActions`                      | OS APIs — winapi `SendInput` on Windows  |
| Works when tab is  | any state, even hidden / not focused             | only when the window is focused & visible|
| Headless           | yes                                              | no                                       |
| Bot detection      | sometimes flagged (`isTrusted=false`)            | hard to detect — events come from OS     |
| Cross-platform     | yes                                              | Windows only (macOS/Linux: stub error)   |
| Dependencies       | none                                             | none — uses `syscall` + `user32.dll`     |

Pick BiDi for general automation. Pick Native when scraping a site that fingerprints synthetic events. Combining `WithHeadless(true)` + `BackendNative` returns `ErrInvalidConfig` — native needs a real window.

## Cursor profile / load

`ProfileHuman` (default) glides along a bezier path with per-point delays — realistic, but each move
can take up to ~1s under heavy concurrency (Firefox runs the pause synchronously before replying).
`WithCursorTuning` lets you shrink `ProfileHuman`'s step cap / delay range without giving up the
bezier shape; `WithCursorProfile` switches profile entirely. Pick based on concurrency:

| Need | Profile | Latency/move (measured) |
|---|---|---|
| low concurrency, human-like | Human (default) | ~0.6–1s |
| medium | Human + `WithCursorTuning(input.CursorTuning{MaxSteps: 30})` | ~0.2–0.4s |
| high (many pages/browsers) | `WithCursorProfile(CursorFast)` | ~0.2s |
| very high / human-likeness not needed | `WithCursorProfile(CursorMachine)` | ~15–30ms |

Figures measured on Firefox 149, headless, single idle page (`integration_stress_move_test.go`,
`-tags stress`). Under fleet load (many pages competing for CPU) latency rises — profile choice is the
main lever you control.

**Scaling shape (measured):** Firefox runs `input.performActions` **in parallel across different
browsing contexts** (20 concurrent pages → ~1.15× a single move, not ~20×). So scale with **more pages
per browser, one goroutine per page** — the real bottleneck is the *per-page* cursor mutex (20
goroutines moving the **same** page serialize to ~14.5× a single move), not a Firefox-side actor.

`CursorTuning` only trades human-likeness for speed — it does not fix CPU oversubscription from
running too many browsers per core; see the load-hardening notes for that.

## Platform support

| OS      | BiDi | Native       |
|---------|------|--------------|
| Windows | yes  | yes          |
| macOS   | yes  | no — stub    |
| Linux   | yes  | no — stub    |

On macOS/Linux, calling a native method returns `input.ErrNotSupported`. Detect and fall back to BiDi if needed.

## Limitations

- **Runtime permission grant unsupported.** Firefox's BiDi does not expose per-origin permission APIs. Use `WithPermissionsDefault` at launch for blanket allow/block. See [docs/permissions-gap.md](docs/permissions-gap.md).
- **CDP unavailable.** Firefox 129+ removed CDP support. Pre-129 builds will not work — use the BiDi-compatible version.
- **DataTransfer-based file upload** may be blocked by sites with strict CSP. Native input backend + OS dialog is a workaround.
- **Native input on macOS/Linux** returns `input.ErrNotSupported`.
- **Native window measurement.** Content-area offset is theme-dependent. Override via `WithNativeWindow(x, y, scale)` — see [docs/native-input-calibration.md](docs/native-input-calibration.md).

## Examples

- [examples/basic](examples/basic/main.go) — BiDi backend, runs anywhere
- [examples/native](examples/native/main.go) — Windows-only, native input

## License

MIT
