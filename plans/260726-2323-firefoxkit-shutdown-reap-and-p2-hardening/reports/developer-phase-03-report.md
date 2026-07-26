# Phase 03 — SetExtraHeaders install/remove race + teardown symmetry

**Status:** completed (TDD, test fail-first đã xác nhận)

## Files
- `concurrency_hardening_test.go` (+110): thêm `TestSetExtraHeaders_SetRacesRemove`,
  `TestSetExtraHeaders_InstallRemoveCycles_NoSubscriptionLeak`, helper `mockInterceptIDs`/`headerState`.
- `page_headers.go` (~+20/-12): commit map + chọn nhánh trong CÙNG section `installMu`; remove thêm
  `session.unsubscribe` + untrack event.
- `page.go` (+20): `trackServerEvent` dedupe; thêm `untrackServerEvent`.

## Fix
1. **Z3-1** — `SetExtraHeaders` lấy `installMu` TRƯỚC, rồi commit `hi.headers` dưới `hi.mu` bên trong,
   đọc luôn `empty`/`installed` cùng section. Caller cuối giữ `installMu` cũng là caller commit cuối
   → trạng thái intercept luôn hội tụ về header set đã commit. Thứ tự khoá `installMu → hi.mu` một
   chiều (cleanup: `initMu → installMu → subsMu`; không có chiều ngược).
2. **Z3-2** — `removeHeaderInterceptor` gỡ tên khỏi `p.serverEvents` + gọi `UnsubscribeEvents` cho
   `network.beforeRequestSent` (chỉ khi có id), trả error đầu tiên nhưng vẫn chạy cả 2 RPC.
   `trackServerEvent` dedupe → N chu kỳ install/remove không phình slice.

## Fail-first (trên code cũ)
```
--- FAIL: TestSetExtraHeaders_InstallRemoveCycles_NoSubscriptionLeak
    cycle 0 after remove: tracked server events = 1, want 0
--- FAIL: TestSetExtraHeaders_SetRacesRemove
    round 28: intercept installed=true with 0 committed headers   <- đúng shape Z3-1
    round 42: intercept installed=false with 1 committed headers
```
Ghi chú: bản race test 2-goroutine/round KHÔNG bắt được bug (commit → lock window quá hẹp). Phải
đẩy 8 map + 8 nil goroutine/round × 50 round để commit-order và installMu-order tách nhau ra.

## Gates
- `go vet ./...` pass · `go build ./...` pass
- `go test -race ./... -count=1` pass (root 20.4s, input, internal/bidi)
- `go test -race -run TestSetExtraHeaders . -count=5` pass (gồm
  `TestSetExtraHeaders_ConcurrentInstall_SingleIntercept` cũ vẫn xanh)

## Hành vi public
Không đổi: map → áp; nil/empty → gỡ. Thêm 1 RPC `session.unsubscribe` mỗi lần gỡ (chủ ý).

## Unresolved
- Z3-5 (`ContinueRequest` dùng `context.Background()`) vẫn còn trong file này — ngoài scope phase 3.
