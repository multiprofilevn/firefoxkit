# Native input: calibrating content offset

The native winapi backend dispatches `SendInput` events at **screen pixel** coordinates. The CSS coordinates your page uses (`getBoundingClientRect`, `(0, 0) = content top-left`) must be mapped to screen pixels via:

```
screen_x = window_x + content_offset_x + css_x * scale
screen_y = window_y + content_offset_y + css_y * scale
```

`window_x/y` is the Firefox window's top-left in screen pixels. `content_offset_x/y` is the distance from the window's top-left to the **content area's** top-left — title bar + tab strip + URL bar + bookmarks bar.

## Default offsets

`input.DefaultFirefoxContentOffsetX = 8`, `DefaultFirefoxContentOffsetY = 104`.

Measured on Firefox 130 ESR / Windows 11 / 100% DPI / default theme / bookmarks bar off. These are coarse defaults — most clicks land within a few pixels of target.

## When defaults are wrong

The chrome-area height changes with:
- Bookmarks bar visible (+24px)
- Compact density (-4px)
- Custom theme with taller toolbar (+0–20px)
- Title bar hidden (system-wide setting)
- Non-100% DPI (Scale knob — see below)

You'll know defaults are off when:
- Clicks consistently miss by a constant offset
- Cursor lands above or below the intended element by the same delta every time

## Recalibration

1. Open Firefox, navigate to a page with a known click target (e.g. a 100x100 div at CSS (200, 200)).
2. Use `input.MeasureContentOffset(hwnd)` to query the OS for `GetClientRect → ClientToScreen`. The returned Y already includes the empirical 104px default. If your visual measurement says the content starts at a different Y, subtract the difference from the default constant in your fork **or** pass an explicit override via `WithNativeWindow`.
3. For a one-shot override, pass the measured offsets directly:

   ```go
   firefoxkit.Launch(path,
       firefoxkit.WithInputBackend(firefoxkit.BackendNative),
       firefoxkit.WithNativeWindow(windowX, windowY, dpiScale),
   )
   ```

   The native backend reads `window.X/Y/Scale` straight from this. Content-offset constants live in `input/backend_native_windows_offset.go`.

## DPI

`Scale` should match the OS display scaling. On Windows: Settings → Display → Scale and layout (100% = 1.0, 125% = 1.25). Wrong Scale causes clicks to drift more the farther they are from the content origin — a tell-tale sign you need to fix Scale rather than the offsets.

## Gotcha: `syscall.NewCallback` exhaustion

The Go runtime maintains a **fixed-size table** for Windows callbacks (~1024 slots on amd64). `syscall.NewCallback` allocates one slot **and never frees it** — there is no GC for these.

If you call `NewCallback` inside a function that runs on every operation (e.g. `findFirefoxHWND` calling `syscall.NewCallback` every `EnumWindows`), the process will panic with `"callback gc table full"` after ~1024 calls.

**Always hoist callbacks to package-level `sync.Once`** + thread state through a mutex-protected package var, not a closure capture:

```go
var (
    findHwndCBOnce sync.Once
    findHwndCB     uintptr
    findHwndMu     sync.Mutex
    findHwndPID    uint32
    findHwndOut    uintptr
)

func findFirefoxHWND(pid uint32) (uintptr, error) {
    findHwndCBOnce.Do(func() {
        findHwndCB = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
            // ... use findHwndPID, write to findHwndOut
        })
    })
    findHwndMu.Lock()
    defer findHwndMu.Unlock()
    findHwndPID = pid
    findHwndOut = 0
    _, _, _ = procEnumWindows.Call(findHwndCB, 0)
    return findHwndOut, nil
}
```

The closure-capture pattern (defining `cb := syscall.NewCallback(func() { ...captures... })` every call) is **always wrong**.

## Across multiple monitors

Firefox's screen coordinates span the virtual desktop. `window_x` can be negative on a secondary monitor positioned to the left of the primary. `pickFreePort` + `Launch` don't position the window — use `WithFlag("-foreground", "")` and let the OS place it, then query `Browser.HWND()` to read `GetWindowRect` for the actual position.
