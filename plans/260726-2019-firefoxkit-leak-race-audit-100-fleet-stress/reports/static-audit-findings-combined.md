# Static audit — bảng finding hợp nhất (Phase 1)

Nguồn: 4 report vùng trong cùng thư mục (`audit-z1-bidi-core.md`, `audit-z2-browser-lifecycle.md`,
`audit-z3-page-layer.md`, `audit-z4-input-package.md`). Chi tiết evidence xem report vùng.
Baseline máy móc: `go vet` + `go build` + `go test -race ./...` (unit, mock) — **toàn bộ xanh**.

## Tổng quan

| Zone | Findings | P1 | P2 | P3 | Ghi chú |
|---|---|---|---|---|---|
| Z1 bidi core | 8 | 0 | 3 | 5 | F1/F2/pending-cleanup re-verify: ĐẠT |
| Z2 browser lifecycle | 10 | 0 | 4 | 6 | drainer fallback + WaitForNewPage là điểm nóng |
| Z3 page layer | 7 | 1 | 2 | 4 | P1 duy nhất: SetExtraHeaders install/remove race |
| Z4 input | 11 | 0 | 6 | 5 | KHÔNG có goroutine nào trong package — rủi ro là shared state |
| **Tổng** | **36** | **1** | **15** | **20** | |

## Finding P1 + P2 (id ổn định, chờ nhãn động từ Phase 3)

| ID | file:line | Loại | Sev | Tóm tắt | Workload run-100 có chạm? |
|---|---|---|---|---|---|
| Z3-1 | page_headers.go:44-65 | race | **P1** | SetExtraHeaders(A) đua SetExtraHeaders(nil) → interceptor bị pin vĩnh viễn, headers rỗng | KHÔNG (workload không dùng headers) → NOT-EXERCISED |
| Z1-1 | transport.go:66-72 | teardown | P2 | writeTimeout 10s cố định không knob; write chậm giữa frame → sập cả session | Gián tiếp (screenshot payload lớn ×100) |
| Z1-2 | events.go:157-168 | event-drop | P2 | Drop network.beforeRequestSent khi queue đầy → request pause treo vĩnh viễn | KHÔNG (không intercept) |
| Z1-3 | events.go:158-159 | unbounded | P2 | Bus key theo method, 1 context bận ăn quota 256 của mọi page | CÓ (tab churn + console) |
| Z2-1 | browser_context_lifecycle.go:64-73 | leak | P2 | Inline fallback khi cleanupCh đầy chặn subscription worker ~30s → drop contextDestroyed → leak b.pages | CÓ (tab churn ×100 browser) |
| Z2-2 | browser_waitpage.go:39-51 | leak | P2 | WaitForNewPage bind context đã chết vào b.pages, không ai gỡ | KHÔNG (workload không dùng WaitForNewPage) |
| Z2-3 | browser.go:140-149 | race | P2 | Close: baseCancel trước signalCleanupDrainer → graceful drain đúng ~50% (path Connect rò server-side sub) | CÓ (close 100 fleet) |
| Z2-4 | browser_waitpage.go:35-56 | timeout | P2 | WaitForNewPage timeout không hard-bound (GetTree rơi backstop 60s) | KHÔNG |
| Z3-2 | page_headers.go:83,133-150 | leak | P2 | Remove interceptor không UnsubscribeEvents → WS traffic không ai tiêu thụ | KHÔNG |
| Z3-3 | page_wait.go:40-47 | waste | P2 | pollUntil poll context đã destroy trọn timeout (RPC mỗi 500ms) | Gián tiếp (nếu tab churn đua wait nội bộ) |
| Z4-1 | input/backend_native_windows.go:157,163-176 | global-race | P2 | nativeInputMu lock per-syscall, 2 browser native chèn gesture nhau | KHÔNG (BiDi backend) |
| Z4-2 | page_mouse_api.go:159-187 | race | P2 | Drag phát actions ngoài mutex cursor engine, đua MoveTo cùng pointer id | KHÔNG (workload không Drag) |
| Z4-3 | page_mouse_api.go:187, input/touch.go:23-27 | state-leak | P2 | Drag/Tap release nút bằng caller ctx — ctx chết → nút kẹt | KHÔNG |
| Z4-4 | input/keyboard_human.go:16,51,61-62 | race | P2 | HumanTypeConfig.Rand shared *rand.Rand không khóa | Gián tiếp (Type dùng config default?) — cook Phase 4 verify |
| Z4-5/6 | cursor_engine.go:57,95-96,141-160 | perf/lock | P2 | Remote path fetch mỗi MoveTo, giữ c.mu suốt 1200ms; DefaultTransport pool=2 | KHÔNG (không set WithMousePathURL) |

P3 (20): xem report vùng — gồm Z1-4 (goroutine bỏ rơi khi timeout drain — fleet 100 tới 100 cặp, CÓ THỂ hiện ra ở run 100), Z2-9 (scanner buffer 64KB/browser ×100), Z2-7 (Close trả lỗi giả os.ErrProcessDone)…

## Câu hỏi mở từ agents (đưa vào Phase 4)

1. Z1-1: nâng P1 nếu số liệu run cho thấy screenshot/upload payload MB-scale.
2. Z2-1: khi drainer bão hòa, có được bỏ hẳn cleanup RPC không (page đã detach)?
3. Z2-3: `b.ctx()` ở nhánh normal drain là cố ý (join nhanh) hay sót?
4. Z4-1: BackendNative có bao giờ chạy >1 Browser đồng thời trong production không?
