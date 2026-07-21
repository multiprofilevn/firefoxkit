---
phase: 4
title: "PERF adaptive cursor knob"
status: pending
priority: P2
effort: "3h"
test-strategy: tdd
dependencies: []
---

# Phase 4: PERF — cursor Human tunable + doc chọn profile theo tải

## Overview
Expose cap step/delay của ProfileHuman (cấu hình được) để ops giảm latency move dưới tải nặng mà
không phải nhảy hẳn sang Fast, + doc map concurrency→profile. Thành thật: knob CHỈ đánh đổi
human-likeness lấy tốc độ; KHÔNG gỡ được CPU oversubscription gốc (200 FF/56 luồng).

## Key Insights (từ audit)
- `input/cursor_path.go:42-52`: `humanStepCount` trần cứng 80 step; `:161` delay `8+rand(11)`ms.
- 1 move Human = tới ~1s (Firefox chạy pause đồng bộ rồi mới trả performActions response).
- Đã có `WithCursorProfile(CursorHuman/Fast/Machine)`. Fast ~10× rẻ, Machine ~vài ms nhưng mất human.
- Chưa có cách hạ trần Human mà giữ dáng bezier.

## Requirements
- Functional: cho phép cấu hình `maxSteps` và dải delay của ProfileHuman qua Options; giá trị mặc
  định GIỮ NGUYÊN hành vi hiện tại (80 step, 8–18ms) → không hồi quy.
- Functional: cap phải được `buildPath`/`humanStepCount`/`withDelays` tôn trọng (số điểm ≤ maxSteps,
  delay trong dải cấu hình).
- Non-functional: KISS — không tự động "adaptive theo CPU" (YAGNI, khó đo đúng, dễ sai); chỉ expose
  tham số tĩnh + doc. Nếu sau này số liệu P5 cho thấy cần auto, mở phase riêng.

## Architecture
- Thêm struct tuning (vd `CursorTuning{ MaxSteps int; MinDelayMs, MaxDelayMs int }`) + option
  `WithCursorTuning(CursorTuning)`; 0-value → default hiện tại.
- Truyền tuning vào `Cursor` (qua `NewCursorWithProfileAndPath` hoặc field). `humanStepCount` nhận
  trần từ tuning thay vì const 80; nhánh Human trong `buildPath` (`cursor_engine.go:160-161`) dùng
  dải delay từ tuning.
- `page_input_backend.go:89` truyền tuning từ `p.browser.cfg`.
- Giữ `Point`/`bezierPath` nguyên (thuật toán không đổi, chỉ tham số hoá số điểm/delay).

## Doc
- Thêm mục ngắn trong README hoặc `docs/` (theo `docs-output-location`: đây là plan active nên nếu
  chỉ là hướng dẫn API sibling thì để trong plan dir; nhưng đây là doc dùng-lâu-dài về profile →
  cập nhật README section "Cursor / input" là hợp lý). Bảng:
  | Concurrency/máy | Profile đề xuất | Latency/move |
  |---|---|---|
  | thấp, cần giống người | Human (default) | ~0.1–1s |
  | trung bình | Human + WithCursorTuning(MaxSteps 30) | ~0.1–0.4s |
  | cao (nhiều luồng/máy) | Fast | ~11–88ms |
  | rất cao / không cần human | Machine | ~vài ms |

## Related Code Files
- Modify: `input/cursor_path.go` (humanStepCount nhận trần), `input/cursor_engine.go` (dải delay),
  `options.go` (`WithCursorTuning` + default), `page_input_backend.go` (truyền tuning).
- Modify: `README.md` (mục Cursor profile guidance).
- Create: `input/cursor_tuning_test.go` (test-first).

## Test Spec (viết TRƯỚC — thuần unit, không cần Firefox)
1. `TestHumanPath_DefaultTuning_Unchanged`: với tuning 0-value, `buildPath` cho khoảng cách lớn tạo
   ≤ 80 điểm, delay mỗi điểm trong [8,18]. Khóa hành vi hiện tại.
2. `TestHumanPath_MaxStepsRespected`: `MaxSteps=20` → path dài nhất ≤ 20 điểm; điểm cuối vẫn ĐÚNG
   `(x2,y2)` (không hỏng invariant terminate).
3. `TestHumanPath_DelayRange`: `MinDelayMs=2,MaxDelayMs=4` → mọi DelayMs ∈ [2,4].
4. `TestTuning_ProfileFastMachine_Untouched`: tuning chỉ ảnh hưởng Human; Fast/Machine giữ nguyên.
5. Regression: `page_mouse_api_cursor_test.go` + input tests xanh.

## Implementation Steps
1. Viết `cursor_tuning_test.go` (đỏ).
2. Tham số hoá humanStepCount + dải delay; thêm tuning field + option + default.
3. Wire qua page_input_backend; cập nhật README bảng profile.
4. `go test ./input/... ./... -race` xanh.

## Success Criteria
- [ ] Default không đổi hành vi (test 1 xanh) — không hồi quy human-likeness.
- [ ] MaxSteps/delay cấu hình được và được tôn trọng; điểm cuối luôn đúng đích.
- [ ] README có bảng hướng dẫn profile theo concurrency.

## Risk Assessment
- Cám dỗ over-engineer "auto-adaptive theo CPU" → CHỐT: không làm trong phase này (YAGNI). Chỉ tham
  số tĩnh. Ghi vào surface-decisions.
- Hạ MaxSteps quá thấp làm path kém giống người → là lựa chọn của ops, doc cảnh báo trade-off.

## Next Steps
Độc lập. Số liệu latency thực đo ở P5 sẽ xác nhận mức MaxSteps đáng đề xuất.
