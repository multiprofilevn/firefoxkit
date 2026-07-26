# Phase 02 — Cleanup drainer drop-on-saturation (Z2-1 + Z2-3)

**Status:** completed · TDD (test đỏ trước, implement sau)

## Files
- `browser_context_lifecycle.go` (+13/-8): nhánh saturated bỏ `target.cleanup(cctx)` inline → drop + `cleanupDropped.Add(1)` + `Warnf(contextID, tổng drop)`. Worker drainer đổi `b.ctx()` → `context.Background()+cleanupTimeout` (khớp `drainRemainingCleanup`).
- `browser.go` (+16/-6): field `cleanupDropped atomic.Uint64`; `Close` gọi `signalCleanupDrainer()` TRƯỚC `baseCancel()`; log tổng drop 1 dòng cuối phase graceful nếu > 0.
- `browser_context_lifecycle_load_test.go` (+58/-6): test mới + chỉnh 1 test cũ (bên dưới).

Không đổi API public.

## Test
- Mới `TestContextDestroyed_DrainerSaturation_DropsCleanup`: 643 page (>`cleanupChCap`+workers), gate `session.unsubscribe` treo → workers kẹt, queue đầy. Assert: mọi context destroy rời `b.pages` ≤3s, `cleanupDropped > 0`, probe event sau flood xử lý ≤500ms. **Chạy trên code cũ: FAIL** (`pages = 124, want 1` — worker kẹt inline 3s/event). Sau fix: PASS 0.15s.
- Inject có pacing 2ms/32 event: cô lập đúng đường bão hòa drainer, tránh lẫn drop-on-full của event queue 256 (đã có test riêng).

## Test cũ chỉnh có chủ đích
`TestCleanupDrainer_StopsOnClose` trước đây dựa vào base-ctx cancel để unblock RPC đang gate → join <2s. Worker giờ dùng Background ctx (đúng ý Z2-3: page nhặt lúc Close vẫn phải tới được peer khỏe), nên join do graceful budget + client teardown/cleanup timeout dẫn dắt. Nới ngưỡng thành `gracefulCloseBudget + cleanupTimeout + 1s` và viết lại comment nêu cơ chế mới. Tính chất cốt lõi giữ nguyên: gate chỉ release SAU khi quan sát join → join vẫn do Close dẫn, không do RPC hoàn tất. Thực đo join ~2.01s.

## Gates
- `go vet ./...` ✅ · `go build ./...` ✅
- `go test -race ./... -count=1` ✅ (root 19.9s, input, internal/bidi)
- `go test -race -run 'TestContextDestroyed|TestCleanupDrainer|TestClose_' -count=5` ✅ (27.8s, không flaky)
- `go vet -tags stress ./...` ✅ · `go build -tags stress ./...` ✅
- `gofmt -l` liệt kê toàn repo (artifact CRLF trên Windows, tồn tại từ trước) — không do thay đổi này.

## Addendum (code review phases 1-5 — F1/F2/F3)

- **F1 đã fix** (chính là concern tự nêu bên dưới): tách `Page.cleanupLocal()` trong `page.go` = nửa không-RPC (claim `cleanedUp` + `Unsubscribe` hdr handler, downloads sub, `p.subs`), một nguồn sự thật — `cleanup()` gọi nó trước rồi mới làm nửa RPC. Nhánh drop trong `browser_context_lifecycle.go` gọi `target.cleanupLocal()` → handler local không còn nằm lại dispatcher bidi giữ `*Page`.
  - Không lấy `installMu` trong `cleanupLocal` (SetExtraHeaders giữ lock đó xuyên qua RPC → sẽ chặn subscription worker). Chỉ lấy `hdr.mu` (bookkeeping thuần). Hệ quả: interceptor commit đúng lúc đang race với drop có thể để lại 1 handler — hiếm, đã ghi comment; đường `cleanup()` bình thường vẫn dọn sạch vì `removeHeaderInterceptor` chạy dưới `installMu`.
  - `cleanup()` giờ snapshot `serverEvents` SAU khi gỡ header interceptor (giữ nguyên thứ tự cũ untrack→snapshot, tránh unsubscribe 2 lần).
  - Test mở rộng: probe page có 1 local sub thật; control "handler còn sống" trước drop → sau drop assert `cleanedUp`, `len(subs)==0`, và inject lại `log.entryAdded` thấy handler KHÔNG chạy. Negative control (comment `cleanupLocal()` ra) → test FAIL đúng chỗ.
- **F2(b) + F3** (comment-only, không đổi hành vi): viết lại comment sweep trong `Close` theo đúng thứ tự thực tế (browser.close → signal drainer → baseCancel → sweep → client.Close → reap) + nêu giá phải trả (op đang bay chậm cancel ≤ `gracefulCloseBudget`, sweep phần lớn fail vì peer đang quit; Connect thì sweep chạy trên peer sống). Doc comment `Close` nêu bound thật ≈ 2s + 2s + `procExitTimeout` ~9-11s worst case.
- **README**: thêm dòng `WithScaleFactor(f)` vào bảng option (cùng style `WithWriteTimeout`).
- Gates chạy lại: `go vet ./...` ✅ · `go build ./...` ✅ · `go test -race ./... -count=1` ✅ · lifecycle/headers `-race -count=5` ✅ · `go vet/build -tags stress` ✅.
- KHÔNG đụng F4-F9 (P3, backlog).

## Concerns
- ~~**Drop bỏ luôn phần cleanup local**~~ → ĐÃ FIX ở addendum (F1): `cleanupLocal()` chạy trên nhánh drop. Còn lại duy nhất mép hẹp: header interceptor commit đúng lúc race với drop (không lấy `installMu` để tránh chặn worker).
- Counter chỉ lộ qua log (đúng yêu cầu "không API public"); nếu sau này cần metric cho fleet thì phải thêm accessor.
