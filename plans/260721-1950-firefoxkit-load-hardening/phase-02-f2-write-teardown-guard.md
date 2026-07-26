---
phase: 2
title: "F2 write-teardown guard"
status: completed
priority: P1
effort: "2h"
test-strategy: tdd
dependencies: []
---

# Phase 2: F2 write-teardown guard (caller cancel không giật sập cả WebSocket)

## Overview
Tách write-ctx khỏi caller-ctx trong `writeJSON`. Hiện tại ctx caller bị cancel/timeout GIỮA lúc
`conn.Write` khiến coder/websocket đóng CẢ connection → mọi Send in-flight của browser đó fail
`ErrClientClosed` → reconnect storm ×200 dưới tải.

## Key Insights (từ audit)
- `internal/bidi/transport.go:49-55`: `t.conn.Write(ctx, ...)` truyền thẳng ctx caller. Comment
  40-48 đã ghi nhận footgun này ("not worked around on purpose — measured in leak-test first"). Giờ
  là lúc xử lý.
- coder/websocket: ctx hủy giữa Write ⇒ đóng nguyên conn (không chỉ hủy write đó).
- Write chỉ là ghi frame cục bộ localhost → nhanh; caller không cần quyền cancel giữa chừng. Response
  -wait mới là chỗ cần tôn trọng caller ctx (giữ nguyên trong `Send`).

## Requirements
- Functional: caller ctx bị cancel/timeout trong lúc `writeJSON` đang chạy KHÔNG được đóng
  connection; các Send in-flight khác trên cùng client vẫn sống và nhận response.
- Functional: write vẫn có giới hạn thời gian riêng (background ctx + `writeTimeout`, đề xuất 10s) để
  một write thật sự kẹt không treo vĩnh viễn.
- Non-functional: caller vẫn hủy được việc CHỜ response (không đổi nhánh `<-ctx.Done()` trong Send).

## Architecture
- `wsTransport.writeJSON`: thay `t.conn.Write(ctx, ...)` bằng ctx nội bộ:
  `wctx, cancel := context.WithTimeout(context.Background(), writeTimeout); defer cancel();
  t.conn.Write(wctx, MessageText, data)`. Caller ctx KHÔNG còn truyền vào Write.
- `writeTimeout` là const/field (đề xuất 10s; có thể tái dùng/độc lập với `defaultCallTimeout` của P1).
- `Send` (client.go): giữ nguyên — response-wait vẫn `case <-ctx.Done()`; chỉ khâu write đổi ctx.
  Lưu ý: sau đổi, một caller ctx đã hủy trước khi Send chạy vẫn nên fail nhanh (đã có guard
  `select { case <-c.closed ...}` + có thể thêm check `ctx.Err()` đầu Send — xác nhận không hồi quy).

## Related Code Files
- Modify: `internal/bidi/transport.go` (writeJSON + const writeTimeout).
- Modify (nếu cần): `internal/bidi/client.go` (đảm bảo caller-ctx-đã-hủy trước Send vẫn fail nhanh,
  không ghi frame thừa).
- Create: `internal/bidi/transport_write_test.go` (test-first, fakeTransport, vào go test mặc định).
- Create: `internal/bidi/transport_write_ws_test.go` (build tag riêng, vd `wsintegration`) — httptest
  WS server THẬT xác nhận đúng hành vi coder/websocket (cancel giữa write KHÔNG teardown). Không vào
  go test mặc định để khỏi làm chậm. **(Validation session 1: user chốt làm CẢ hai.)**

## Test Spec (viết TRƯỚC)
1. `TestWrite_CallerCancelMidWrite_ConnSurvives` (root package, httptest WS server chậm accept
   write / hoặc fakeTransport có hook): chạy 1 Send với ctx sắp hết hạn trong lúc write, đồng thời
   1 Send khác ctx rộng. Assert: Send thứ hai vẫn nhận được response (conn không bị đóng);
   client chưa `closed`.
2. `TestWrite_StuckWrite_BoundedByWriteTimeout`: transport giả treo write → writeJSON return lỗi sau
   ~writeTimeout, KHÔNG treo vĩnh viễn.
3. `TestSend_CallerCtxCanceledBeforeWrite_FailsFast`: ctx đã cancel trước khi Send → return nhanh,
   không ghi frame lên transport (assert `writeJSON` call count = 0 hoặc lỗi ctx trả ngay).
4. Regression: `internal/bidi` + root xanh, `-race` sạch; đặc biệt các test Close/drain hiện có.

## Implementation Steps
1. Viết test theo Test Spec (đỏ). Với case 1, ưu tiên fakeTransport có cờ `blockWriteUntil` +
   `closeOnCtxCancel` để mô phỏng coder/websocket mà không cần WS thật.
2. Sửa `writeJSON` dùng background ctx + writeTimeout.
3. (Nếu cần) thêm fail-fast check caller ctx đầu Send.
4. Chạy `go test ./internal/bidi/... ./... -race` xanh.

## Success Criteria
- [ ] Caller cancel/timeout giữa write KHÔNG đóng connection; Send khác sống sót (test 1 xanh).
- [ ] Write có trần thời gian riêng (test 2 xanh).
- [ ] Có 1 httptest WS thật (tag `wsintegration`) xác nhận không teardown — xanh.
- [ ] Không hồi quy Close/drain; `-race` sạch.

## Risk Assessment
- Mất khả năng caller hủy write giữa chừng: chấp nhận được (write localhost nhanh; có writeTimeout
  10s backstop). Doc ghi rõ trade-off.
- Nếu fakeTransport không mô phỏng đủ sát hành vi coder/websocket → bổ sung 1 test tích hợp WS thật
  (httptest) đánh dấu build tag phù hợp để không làm chậm `go test` mặc định.

## Next Steps
Độc lập P1/P3/P4.
