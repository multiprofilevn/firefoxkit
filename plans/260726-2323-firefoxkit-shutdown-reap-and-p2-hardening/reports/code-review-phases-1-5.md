# Code review — phases 1-5 (shutdown reap + P2 hardening)

Scope: `git diff HEAD` + 10 file untracked (~594 insert / 18+10 file). Read-only.

## Gate results

| Gate | Kết quả |
|---|---|
| `go vet ./...` (windows) | clean |
| `CGO_ENABLED=0 GOOS=linux/darwin go build ./...` | OK (stub `browser_process_tree_other.go` compile) |
| `GOOS=linux go vet`, `go vet -tags stress` | clean |
| `go test -race ./...` | PASS (fk 20.4s, input 5.6s, bidi 18.8s) |
| `-race -count=3` trên test mới | PASS, không flaky |
| Plan/finding ID rò vào comment | không (chỉ 2 comment `phase-05 spec` **có sẵn từ trước** ở `integration_stress_move_test.go:13,200`) |
| `gofmt -l` | liệt kê toàn repo → artifact CRLF sẵn có, không phải finding |

## Findings

| ID | file:line | Sev | Evidence | Fix đề xuất | Conf |
|---|---|---|---|---|---|
| F1 | `browser_context_lifecycle.go:64-78` | P2 | Nhánh drop chỉ detach khỏi `b.pages` rồi bỏ hẳn `p.cleanup`. Nhưng local subscription handler chỉ được gỡ trong `page.go:164-174` (`p.subs` → `Unsubscribe`) + `page_headers.go:149-151`. Page đã rời `b.pages` nên `Close` (`browser.go:207`) cũng không quét nó → handler ở lại dispatcher bidi tới hết đời browser, giữ luôn `*Page` trong closure. Comment ":71-73 subscriptions … die with it" chỉ đúng cho server-side. | Trên nhánh drop làm phần **không RPC**: `p.cleanedUp.Swap(true)` + `Unsubscribe()` local subs (µs, không block worker); vẫn bỏ RPC như user đã duyệt | 90% |
| F2 | `browser.go:164-191` vs `197-201` | P2 | `browser.close` giờ chạy TRƯỚC `baseCancel()` và trước sweep `p.cleanup`. (a) op user đang bay không còn được cancel ngay — chạy thêm tới 2s trên browser vừa bị bảo đóng (contract cũ ":187-188 Cancel the base context so any in-flight user operation … unblocks" bị nới 2s). (b) Comment ":198-200 RPCs still succeed on a healthy peer" đã **stale**: peer đã nhận lệnh close 2s trước → sweep phần lớn fail + sinh log warn. | Gửi `browser.close` SAU sweep page-cleanup (hoặc ít nhất sau `baseCancel`); nếu giữ thứ tự hiện tại thì sửa comment :198-200 | 85% |
| F3 | `browser.go:154-241` | P2 | Worst-case Close = 2s graceful + 2s sweep + join `<-gracefulQuit` + 5s `procExitTimeout` ≈ 9-11s, vượt non-functional phase 1 "Close vẫn bounded (~≤ graceful + 5s)". Vẫn bounded, chỉ là con số trong spec/doc không còn đúng. | Cập nhật doc/spec hoặc gộp budget graceful chung 1 deadline | 90% |
| F4 | `browser.go:220` | P3 | `err != nil && firstErr == nil && gracefulQuit == nil` — `gracefulQuit` luôn non-nil khi `closeProc && client != nil`, tức **mọi** lỗi teardown transport trên đường Launch bị nuốt vĩnh viễn, kể cả khi `CloseBrowser` fail. | Gate theo "graceful RPC thực sự thành công" (ghi kết quả vào biến) thay vì theo channel có tồn tại hay không | 85% |
| F5 | `browser.go:255-259` | P3 | Nhánh `procDone == nil` không thể đạt tới ở production (`closeProc:true` chỉ ở `browser_launch.go:136`, luôn kèm `procDone` :141). Nhánh này trả `b.proc.Kill()` thô → tái sinh Z2-7 (không lọc `os.ErrProcessDone`) và không wrap pid. | Bỏ nhánh, hoặc cho test dùng `procDone` đã close (test hiện tại đều set procDone) | 90% |
| F6 | `page_headers.go:158-165` | P3 | `untrackServerEvent` chạy TRƯỚC `UnsubscribeEvents`; RPC fail → tên event đã bị gỡ khỏi `p.serverEvents` nên `p.cleanup` (`page.go:177-183`) không retry → sub server-side sống tới khi context chết. | Untrack sau khi unsubscribe thành công | 80% |
| F7 | `browser_process_tree_windows.go:104-127` | P3 | `reap()` set `r.job=0` rồi lần gọi sau rơi xuống nhánh `taskkill` theo **pid** — pid có thể đã bị OS tái sử dụng. Hiện không có đường gọi 2 lần (`cleanup()` ở `browser_launch.go:89-94` chỉ dùng trước khi Wait-owner chạy; `Close` gọi 1 lần dưới `closeOnce`), nên latent. Ngoài ra job handle rò nếu Browser không bao giờ `Close`. | Thêm cờ `done` để reap thứ 2 là no-op tuyệt đối | 75% |
| F8 | `input/gesture.go:19-26` | P3 | `WithGesture`/`ReleaseCtx` export nhưng `gestureLocker` unexported → Backend do user tự viết (interface `input.Backend` là public) không bao giờ tham gia được gesture lock. | Nếu cố ý, ghi rõ 1 dòng trong doc `WithGesture` | 85% |
| F9 | `page_mouse_api.go:104-118` | P3 | `DoubleClick` = 2 lần `clickButton` → 2 gesture rời; browser native khác chen được vào giữa 2 click, phá dblclick. Cùng lớp lỗi Z4-1 nhưng ngoài scope phase 5 đã chốt. | Bọc cả 2 click trong 1 `input.WithGesture` (backlog) | 85% |

Re-entrancy marker: **không có rủi ro sống**. Grep `go func` trong `input/*.go` + `page_mouse_api.go` = 0 → không op nào spawn goroutine re-enter `WithGesture` bằng ctx khác. Nesting thực tế duy nhất (`Drag` → `cursorEngine().MoveTo`) truyền đúng ctx đã đánh dấu (`page_mouse_api.go:174,180`) → pass-through, không self-deadlock. Ràng buộc "fn phải truyền ctx xuống" mới chỉ là contract trong doc (`input/gesture.go:36-38`).

Lock order: `installMu → hi.mu` một chiều (`page_headers.go:50-60`, `page.go:136-140`); `gesture → c.mu → nativeInputMu` (`input/cursor_engine.go:93-101,199-220`, `input/backend_native_windows.go:17-23`). Không tìm thấy đường nào giữ `c.mu`/`nativeInputMu` rồi mới xin gesture → **không có inversion**. `pagesMu` chỉ giữ trong đoạn ngắn không gọi RPC (`browser.go:192-196`).

Regression walk: `Connect` chỉ nhận thêm `WriteTimeout` (`browser_connect.go:37`), `closeProc:false` → không chạm graceful quit / job object / reap. Double-Close vẫn an toàn qua `closeOnce`. Close-from-handler: `internal/bidi/close_from_handler_test.go` xanh (Close từ handler nay tốn thêm ≤2s vì reader kẹt trong handler, vẫn bounded). Non-Windows: stub là type thật, `reap()` no-op, `reapProcess` giờ lọc `os.ErrProcessDone` → **hết lỗi giả Z2-7**, đổi nhẹ theo hướng tốt. Z2-8 double-Wait đã hết thật: `cleanup()` chỉ dùng ở `browser_launch.go:101,112,127`, đều trước goroutine Wait-owner (`:149`).

Public API: `WithWriteTimeout` (âm bị bỏ qua, 0 = default 10s — khớp `transport.go:72-77`), `WithScaleFactor` (≤0 bỏ qua, `strconv.FormatFloat 'g'` → "1.5"/"2"), `input.WithGesture`, `input.ReleaseCtx`. Không đổi chữ ký nào sẵn có; README bổ sung `WithWriteTimeout` (`WithScaleFactor` **chưa** có trong bảng README — nên thêm).

## Phase acceptance

| Phase | Verdict | Ghi chú |
|---|---|---|
| 1 — process-tree reap | **PASS*** | 5 unit test reap/verify-dead xanh; vet/build xanh; nhánh survivor có test thật. (*) tiêu chí "smoke N=2 headed 0 orphan" chưa chạy ở review này → thuộc phase 6. F3/F4/F5 không chặn. |
| 2 — drainer drop | **PASS với điều kiện** | Drop + counter + log ✓, không còn cleanup trên subscription worker ✓, thứ tự signal→cancel ✓, drainer ctx = Background ✓. **F1 phải xử lý** trước khi coi là đóng (rò handler local, mâu thuẫn với comment). |
| 3 — SetExtraHeaders | **PASS** | Commit trong `installMu`, rẽ nhánh theo `len(hi.headers)` ✓; unsubscribe + dedupe ✓; test cân bằng add/remove ×N ✓. F6 là mép nhỏ. |
| 4 — write timeout | **PASS** | Plumb đủ `options → Launch/Connect → bidi.Options → transport` ✓, default không đổi ✓, 4 test transport + 4 test option ✓, doc nêu trade-off mutex ✓. |
| 5 — gesture lock + scale | **PASS** | Gesture-scope lock chia sẻ process-wide ✓ (test 2 instance khác nhau vẫn contend), BiDi backend giữ song song ✓, Drag/Tap release qua `ReleaseCtx` ✓, pref `layout.css.devPixelsPerPx` ✓ + doc liên kết 3 API. F9 ngoài scope đã chốt. |

## Điểm tốt

Test là test thật (assert hai chiều, có case "test không chứng minh gì" tự fail — `concurrency_hardening_test.go:51`); comment giải thích *why*/invariant chứ không kể lại code; không tạo abstraction thừa; reap idempotent + fallback taskkill có kiểm tra `processAlive` hai lần để tránh báo lỗi giả.

## Unresolved questions

1. F1 — chấp nhận rò handler local trên nhánh drop, hay thêm teardown local không-RPC?
2. F2 — thứ tự `browser.close` trước sweep là cố ý hay nên đảo? (ảnh hưởng contract cancel của `Close`)
3. `WithScaleFactor` có nên vào bảng option trong README như `WithWriteTimeout` không?

**Status:** DONE
