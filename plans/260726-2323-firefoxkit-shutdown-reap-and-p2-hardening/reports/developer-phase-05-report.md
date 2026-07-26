# Phase 05 — Native gesture lock + scale-factor pref

**Status:** completed · TDD (red → green cả 3 sub-fix)

## Files

- Created: `input/gesture.go` (60 LOC), `input/gesture_test.go`,
  `input/backend_native_gesture_lock_test.go` (windows), `page_mouse_api_drag_release_test.go`
- Modified: `input/cursor_engine.go` (MoveTo/Click/Scroll), `input/touch.go` (Tap),
  `input/backend_native_windows.go` (+lockGesture/unlockGesture, doc lock order),
  `page_mouse_api.go` (Drag, Scroll), `options.go` (WithScaleFactor), `options_prefs_test.go`

## Z4-1 gesture-scope lock

- `WithGesture(ctx, backend, fn)` + unexported `gestureLocker` iface; native backend implement,
  BiDi KHÔNG (giữ song song). Mutex `nativeGestureMu` **package-level** → mọi native backend
  (9 browser) chung 1 lock; `nativeInputMu` per-syscall giữ nguyên, lock order gesture → input.
- Re-entrancy qua ctx marker (`gestureHeldKey`): Drag giữ gesture rồi gọi `Cursor.MoveTo` →
  pass-through, không deadlock (test `TestGestureLock_DragStyleNesting`).
- Wrap: `Cursor.MoveTo/Click/Scroll`, `TouchScreen.Tap`, `MouseAPI.Drag/Scroll`.
- **Red confirm** (trước fix): `gesture A#1 interleaved: 3 ops spread over 6 slots`,
  stream `[{B#0 move} {B#0 down} {A#0 move} {A#0 down} {B#0 up} …]` — đúng bug Z4-1.

## Z4-3 button-release safety

- `input.ReleaseCtx(ctx)`: ctx sống → pass-through; ctx chết → background 2s. Dùng ở `Tap`,
  `Drag` (cả error path lẫn release cuối), và `Cursor.Click` (thay 4 dòng inline — DRY, cùng budget).
- Red confirm: `Tap`/`Drag` fail "released on the cancelled caller ctx" trước fix, pass sau.

## WithScaleFactor

- `options.go`: pref `layout.css.devPixelsPerPx` = string (`strconv.FormatFloat 'g'` → "1.5"/"2"),
  f <= 0 → không ghi pref (Firefox default -1.0 = system DPI). Doc nêu quan hệ với
  `WithNativeWindow(x,y,scale)` (map tọa độ native) + `SetWindowBounds` (tile), và pref chỉ áp lúc launch.

## Gates

`go vet ./...` ✓ · `go build ./...` ✓ · `go test -race ./... -count=1` ✓ (root 20.4s, input 5.0s, bidi 18.1s)
· `go test -race ./input -count=5` ✓ · `go vet/build -tags stress ./...` ✓ · gofmt sạch trên file đã đụng
(`options.go` lệch align sẵn từ HEAD — không đụng).

## Quyết định tự chọn

- Wrap thêm `MouseAPI.Scroll` (ngoài spec: chỉ ghi Click/Drag/Tap): wheel native rơi vào vị trí con
  trỏ chung → cùng class lỗi Z4-1. Origin resolve TRƯỚC lock (scrollOrigin có thể gọi BiDi Evaluate).
- Không wrap keyboard path — Z4-1 là lỗi tọa độ; `nativeInputMu` đã đủ cho key. Ghi backlog nếu cần.
- Không đụng Z4-2 (backlog theo spec).
