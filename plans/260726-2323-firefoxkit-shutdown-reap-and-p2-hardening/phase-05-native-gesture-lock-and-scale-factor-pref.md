---
phase: 5
title: "Native gesture lock and scale-factor pref"
status: completed
priority: P2
effort: "3h"
dependencies: []
---

# Phase 5: Native gesture lock and scale-factor pref

## Overview

Fix cụm Z4 cho kịch bản production **native backend ≤ 9 browser đồng thời** (user xác nhận — chuột
thật, cửa sổ tile bằng `SetWindowBounds` + `WithNativeWindow`). Z4-1: `nativeInputMu` lock theo
**từng syscall** → 2 browser native chèn gesture nhau (`SetCursorPos` của B lọt giữa move và
button-down của A → click A rơi vào cửa sổ B). Kèm Z4-3 (release nút bằng caller ctx → nút kẹt)
và tiện ích scale-factor pref user yêu cầu (tương đương chrome `--scale-factor`).

## Requirements

- Functional:
  1. **Gesture-scope lock** ([input/backend_native_windows.go:157,163-176,313](../../input/backend_native_windows.go)): giữ `nativeInputMu` trọn gesture (move→down→up / move→click / drag sequence), không per-syscall. API nội bộ dạng `withGesture(func())` hoặc nâng các hàm composite (Click/Drag/Tap path) acquire một lần. BiDi backend KHÔNG bị ảnh hưởng (lock chỉ ở native).
  2. Z4-3: `Drag`/`Tap` release nút bằng background ctx ~2s như `Cursor.Click` đã làm đúng ([input/cursor_engine.go:203-210](../../input/cursor_engine.go)) — hết kẹt nút khi caller ctx chết ([page_mouse_api.go:187](../../page_mouse_api.go), [input/touch.go:23-27](../../input/touch.go)).
  3. **`WithScaleFactor(f float64)`** trong `options.go` → pref `layout.css.devPixelsPerPx` (options_prefs pipeline) — tương đương chrome scale-factor; doc ghi rõ tương tác với `WithNativeWindow(x, y, scale)` (scale tọa độ native) và `SetWindowBounds` (tile).
- Non-functional: không đổi hành vi single-browser; throughput native multi-browser chấp nhận serialize theo gesture (bản chất 1 con trỏ OS/desktop).

## Related Code Files

- Modify: `input/backend_native_windows.go`, `page_mouse_api.go` (Drag), `input/touch.go` (Tap), `options.go`, `options_prefs.go`
- Modify (test): `input/backend_fake_test.go` pattern → test mới `input/backend_native_gesture_lock_test.go`; `options_prefs_test.go` (scale pref)

## Existing code audit

Audit Z4 (`../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z4-input-package.md`, Z4-1/Z4-2/Z4-3) map đủ vùng — thay vai trò reuse-scout. Window surface ĐÃ CÓ: `WithNativeWindow(x,y,scale)` (options.go:105), `SetWindowBounds` (browser_focus_windows.go:171 — doc ghi rõ mục đích tile native multi-profile), `WindowScale()`. Verdict: **REUSE-EXTEND** lock hiện có thành gesture-scope; **REUSE-AS-IS** window APIs; **FORK-NEW** chỉ mỗi option scale pref (theo pattern prefs sẵn có).

## Reuse strategy

Không viết window-manager mới — tile là việc của caller qua `SetWindowBounds` (đã có). Chỉ fix atomicity gesture + convenience pref. Z4-2 (Drag ngoài mutex cursor engine, đua MoveTo cùng pointer id BiDi) ghi backlog — khác backend, cần thiết kế riêng.

## Implementation Steps (TDD)

1. **Test trước** — gesture atomicity với fake syscall layer (pattern `backend_fake_test.go`): 2 goroutine mô phỏng 2 browser native bắn gesture xen kẽ → assert chuỗi sự kiện ghi lại không interleave trong 1 gesture (move,down,up liền khối). Test này FAIL trên lock per-syscall hiện tại.
2. Implement gesture lock; chạy lại test 1.
3. Test + fix Z4-3: cancel caller ctx giữa Drag → assert button-up vẫn được gửi (fake backend ghi nhận up).
4. Test pref: `WithScaleFactor(1.5)` → profile prefs chứa `layout.css.devPixelsPerPx="1.5"`; implement option.
5. `go test -race ./...` (đặc biệt package `input/`).

## Success Criteria

- [x] Test interleave FAIL trên code cũ, PASS sau fix (xác nhận cả 2 chiều)
- [x] Drag/Tap release nút dù caller ctx chết
- [x] `WithScaleFactor` ra đúng pref; doc 3 API window/scale liên kết nhau rõ
- [x] `go test -race ./...` xanh

## Risk Assessment

- Gesture lock giữ lâu (ProfileHuman move ~1s) → serialize 9 browser native là chấp nhận (1 con trỏ OS); doc nêu rõ để caller không kỳ vọng parallel native.
- `layout.css.devPixelsPerPx` áp per-profile lúc launch, không đổi runtime — doc ghi rõ.
