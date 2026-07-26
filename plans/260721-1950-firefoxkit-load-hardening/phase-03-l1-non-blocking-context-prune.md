---
phase: 3
title: "L1 non-blocking context prune"
status: completed
priority: P1
effort: "4h"
test-strategy: tdd
dependencies: []
---

# Phase 3: L1 non-blocking contextDestroyed prune (chống leak b.pages dưới tab churn)

## Overview
Tách phần gỡ `b.pages` (rẻ) khỏi phần `cleanup()` (RPC chặn 3s) trong handler `contextDestroyed`.
Hiện handler chạy cleanup chặn TRÊN subscription worker (1 worker/queue-256); tab/popup churn dồn dập
làm queue tràn → event bị DROP → page không được gỡ khỏi `b.pages` → leak tuyến tính.

## Key Insights (từ audit)
- `browser_context_lifecycle.go:23-33`: 1 subscription session-wide cho `contextDestroyed`.
- `internal/bidi/events.go:157-168`: dispatch DROP non-blocking khi queue đầy (cap 256, 1 worker).
- `removeDestroyedPage` (`:37-69`): phần gỡ `b.pages` dưới `pagesMu` là RẺ; phần chặn là
  `target.cleanup(cctx)` (UnsubscribeEvents/RemoveIntercept/input Release, ctx 3s) — chạy trên worker.
- Mọi destroy top-level LẪN child-frame (iframe SPA) đều bơm qua 1 subscription này.

## Requirements
- Functional: dưới flood N (vd 1000) `contextDestroyed` với cleanup chậm, TẤT CẢ page tương ứng phải
  bị gỡ khỏi `b.pages` (không drop) — invariant chống-leak không phụ thuộc tốc độ cleanup.
- Functional: cleanup RPC vẫn chạy (best-effort) nhưng KHÔNG chặn việc drain event.
- Non-functional: KHÔNG goroutine-per-event (tránh anti-pattern L2). Dùng drainer bounded.
- Non-functional: mọi goroutine drainer chết khi Browser.Close (không leak worker).

## Architecture
- Trong handler: chỉ gọi phần "detach" (gỡ khỏi `b.pages`, cập nhật `b.current`) — đồng bộ, rẻ. Sau
  đó đẩy `target` vào một `cleanupCh chan *Page` có buffer (vd 512).
- Một cleanup drainer: N worker nhỏ (vd 2–4) đọc `cleanupCh`, chạy `target.cleanup(cctx)` với ctx 3s.
  Khởi tạo cùng `watchContextLifecycle`; dừng khi Browser.Close (đóng `cleanupCh` hoặc qua done ctx +
  wg để Close join, không để worker sống lâu hơn browser).
- Nếu `cleanupCh` đầy (cực hiếm): fallback đồng bộ cleanup NGAY trong handler cho case đó (đảm bảo
  không mất cleanup) — nhưng detach vẫn đã xong nên `b.pages` không leak dù cleanup có chậm.
- Tách hàm: `detachDestroyedPage(id) *Page` (phần rẻ, trả target để cleanup) + drainer riêng. Giữ
  `removeDestroyedPage` như wrapper nếu nơi khác gọi (kiểm bằng grep).

## Related Code Files
- Modify: `browser_context_lifecycle.go` (tách detach vs cleanup, thêm drainer + lifecycle).
- Modify (nếu drainer field ở Browser): `browser.go` (field cleanupCh/wg + dừng trong Close path
  `browser.go:137-140`).
- Create: `browser_context_lifecycle_load_test.go` (test-first, mock harness).

## Test Spec (viết TRƯỚC — dùng newMockBrowser + injectEvent)
1. `TestContextDestroyed_Flood_NoPageLeak`: mock browser với M page (contextID biết trước); mock
   `cleanup` RPC (UnsubscribeEvents/RemoveIntercept) handle chậm (block ~50ms/each). Bắn M event
   `contextDestroyed` liên tiếp nhanh (vượt tốc độ drain). Assert: sau settle, `len(b.pages)==0`
   (KHÔNG drop event detach) — đây là bằng chứng leak được vá.
2. `TestContextDestroyed_Detach_IsSynchronousAndCheap`: bắn 1 event → `b.pages` gỡ NGAY (trước khi
   cleanup RPC hoàn tất); assert page mất khỏi `Pages()` mà không cần chờ cleanup.
3. `TestContextDestroyed_UnknownContext_NoOp`: event với context lạ (child frame) → không panic,
   `b.pages` không đổi.
4. `TestCleanupDrainer_StopsOnClose`: sau `Browser.Close`, goroutine drainer trở về baseline
   (`runtime.NumGoroutine` settle) — không leak worker.
5. Regression: `TestStress_TabChurn_NoPageLeak` (integration_stress_test.go) + concurrency tests xanh.

## Implementation Steps
1. Viết test theo Test Spec (đỏ) — đặc biệt test 1 phải FAIL với code hiện tại (chứng minh bug).
2. Tách `detachDestroyedPage` (rẻ) khỏi cleanup; thêm `cleanupCh` + drainer worker pool + wiring
   Close.
3. Handler mới: detach đồng bộ → enqueue cleanup (fallback đồng bộ nếu buffer đầy).
4. `go test ./... -race` xanh; chạy lại mock flood test nhiều lần chống flaky.

## Success Criteria
- [ ] Test 1 (flood) chuyển từ đỏ→xanh: `b.pages` không leak dưới churn dồn dập.
- [ ] Detach đồng bộ, cleanup offload không chặn drain (test 2).
- [ ] Drainer không sống lâu hơn browser (test 4); `-race` sạch.
- [ ] Không hồi quy tab-churn/concurrency hiện có.

## Risk Assessment
- Đổi thứ tự: cleanup giờ chạy SAU detach, có thể trễ hơn → RPC tới context đã chết vẫn fail vô hại
  (đã đúng như comment cũ). Không đổi ngữ nghĩa quan sát được.
- Buffer đầy fallback đồng bộ: hiếm, nhưng giữ để không bao giờ mất cleanup. Doc rõ.
- Cẩn thận race giữa detach (pagesMu) và các đường close khác (`cleanedUp` idempotent đã có) — giữ
  `cleanup` idempotent, không double-free.

## Next Steps
Độc lập P1/P2/P4.
