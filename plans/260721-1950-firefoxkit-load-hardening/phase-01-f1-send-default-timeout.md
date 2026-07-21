---
phase: 1
title: "F1 Send default timeout"
status: pending
priority: P1
effort: "3h"
test-strategy: tdd
dependencies: []
---

# Phase 1: F1 Send default timeout (backstop khi caller ctx không deadline)

## Overview
Thêm backstop timeout ở `bidi.Client.Send`: khi caller ctx KHÔNG có deadline, Send tự áp một deadline
mặc định (cấu hình được). Chặn kịch bản conn còn sống nhưng Firefox mất 1 response → goroutine park +
entry `pending` treo suốt đời browser → tích lũy ×200 browser × nhiều giờ.

## Key Insights (từ audit)
- Base ctx browser: `browser.go:52` `context.WithCancel(context.Background())` — không deadline.
- Nhiều call `Send` không timeout: `browser_pages.go:34,50,88,106,151`, `page.go:275,281,287`,
  `browser_waitpage.go:35`, `browser_webextension.go:16`.
- `Send` (`internal/bidi/client.go:107-135`) chỉ thoát khi có response, `ctx.Done()` (không bao giờ
  nếu ctx không deadline), hoặc `c.closed` (chỉ khi Browser.Close). → park.
- `defer c.cancelPending(id)` (client.go:100) đã dọn map khi Send return → chỉ cần Send return được.

## Requirements
- Functional: một Send với caller ctx không deadline, khi response không bao giờ tới trên conn còn
  sống, PHẢI return `ErrCommandTimeout` trong khoảng backstop và giải phóng goroutine + xóa entry
  `pending`.
- Functional: caller ctx CÓ deadline riêng → tôn trọng deadline đó, KHÔNG override.
- Non-functional: backstop mặc định rộng (≥ 60s, đề xuất 60s) để không cắt RPC hợp lệ dài nhất
  (performActions human ~1s, screenshot/nav vài s). Cấu hình được qua Options.
- Non-functional: không đổi chữ ký public của `Send`; không sửa rải rác từng caller.

## Architecture
- `bidi.Options`/`Client` thêm field `defaultCallTimeout time.Duration` (0 = tắt backstop, giữ hành
  vi cũ cho ai muốn). Set từ firefoxkit config (mặc định 60s).
- Trong `Send`, đầu hàm: nếu `defaultCallTimeout > 0` và `ctx` không có deadline
  (`_, ok := ctx.Deadline(); !ok`) → `ctx, cancel = context.WithTimeout(ctx, d); defer cancel()`.
- Giữ nguyên phần còn lại: write + select. Wrap `ErrCommandTimeout` đã có sẵn ở nhánh
  `context.DeadlineExceeded` (client.go:129-131) → caller `errors.Is` được.
- Expose option firefoxkit: `WithCommandTimeout(d time.Duration)` trong `options.go`; truyền xuống
  `bidi.Dial`/`newClient`. Mặc định trong `defaultConfig()`.

## Related Code Files
- Modify: `internal/bidi/client.go` (field + logic đầu Send), `internal/bidi/transport.go` hoặc nơi
  build Options (truyền timeout).
- Modify: `options.go` (`WithCommandTimeout` + default), nơi `bidi.Dial` được gọi (`browser_connect.go`
  / `browser_launch.go`).
- Create: `internal/bidi/client_timeout_test.go` (test-first).

## Test Spec (viết TRƯỚC — dùng fakeTransport)
1. `TestSend_NoDeadline_BackstopTimeout`: `newClient` với `defaultCallTimeout=50ms` + `fakeTransport`
   không bao giờ trả response cho id. Gọi `Send(context.Background(), ...)`. Assert: return trong
   ~<200ms với `errors.Is(err, ErrCommandTimeout)`; `len(pending)==0` sau đó; số goroutine trở về
   baseline (đo bằng `runtime.NumGoroutine` sau settle).
2. `TestSend_CallerDeadline_NotOverridden`: caller ctx deadline 20ms, backstop 10s → return theo 20ms
   (không phải 10s). Assert timing < 500ms.
3. `TestSend_BackstopDisabled_LegacyPark`: `defaultCallTimeout=0` + background ctx → Send KHÔNG return
   cho đến khi `Close()` (giữ hành vi cũ); assert `Close` giải phóng qua `drainPending` (ErrClientClosed).
4. Regression: mọi test `internal/bidi` + root hiện có xanh; `-race` sạch.

## Implementation Steps
1. Viết `client_timeout_test.go` theo Test Spec (đỏ trước).
2. Thêm `defaultCallTimeout` vào Client + Options; wire qua `newClient`/`Dial`.
3. Thêm nhánh backstop đầu `Send`.
4. Thêm `WithCommandTimeout` + default 60s trong firefoxkit `options.go`; wire vào Dial call site.
5. `go test ./internal/bidi/... -race` + `go test ./... -race` (buildvcs=false nếu cần) xanh.

## Success Criteria
- [ ] 3 test Test Spec xanh + regression xanh, `-race` sạch.
- [ ] Caller có deadline không bị override; backstop chỉ áp khi thiếu deadline.
- [ ] Default 60s wired end-to-end; `WithCommandTimeout` override được.

## Risk Assessment
- Backstop quá ngắn cắt RPC hợp lệ dài → chọn 60s, doc rõ; performActions dài nhất ~1s « 60s.
- Một số flow chờ-lâu (nav chờ event) KHÔNG chờ bằng blocking Send mà qua event/wait riêng → không
  bị 60s cắt. Xác minh khi cook: liệt kê caller thực sự block-Send lâu > 60s (kỳ vọng: không có).

## Next Steps
Độc lập P2/P3/P4. Không chặn phase nào.
