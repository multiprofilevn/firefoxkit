# Phase 01 — Process-tree reap + verify-dead in Close

**Status:** completed · TDD (test đỏ trước, impl sau)

## Files

**Created**
- `browser_process_tree_windows.go` (175 LOC) — Job Object `KILL_ON_JOB_CLOSE` + fallback `taskkill /T /F`, `processAlive`
- `browser_process_tree_other.go` (12 LOC) — stub no-op `//go:build !windows`
- `browser_process_tree_windows_test.go` (190 LOC) — 5 test
- `internal/bidi/browser_close.go` (11 LOC) — `Client.CloseBrowser` → `browser.close`, params rỗng

**Modified**
- `browser.go` (+62/−6) — field `procTree`/`procDone`, var `procExitTimeout=5s`, graceful `browser.close` đầu Close (goroutine + budget 2s, Launch-only), method `reapProcess`, bỏ double-Wait
- `browser_launch.go` (+16/−5) — tạo reaper ngay sau `cmd.Start()` + log mode, `cleanup` reap tree, goroutine `cmd.Wait` là single owner và `close(procDone)`

## Hành vi mới

1. Close (Launch): `browser.close` best-effort ≤2s → cleanup sweep → client.Close → reap tree (job handle close, fallback taskkill) → `proc.Kill()` backstop → chờ `procDone` ≤5s. Timeout → `firefoxkit: firefox pid=N survived kill`.
2. Kill "thành công" nhưng process còn sống KHÔNG còn bị nuốt — chỉ tín hiệu `cmd.Wait` mới tính là chết.
3. Connect path không đổi: không gửi `browser.close`, không reap.
4. Lỗi `client.Close` sau graceful quit bị bỏ qua (peer tự drop socket) → hết lỗi giả khi đóng.

## Verify

- `go vet ./...` · `go build ./...` — pass
- `go test -race ./... -count=1` — pass (4 package)
- `go vet -tags stress ./...` · `go build -tags stress ./...` — pass
- `CGO_ENABLED=0 GOOS=linux|darwin go build ./...`, `GOOS=linux go vet ./...` — pass (stub xanh)
- Unit mới: job kill tree (3.06s), taskkill fallback kill tree (3.33s), already-dead → nil (0.08s), reapProcess happy (3.04s), reapProcess survivor-error (0.21s) — 5/5 PASS
- Smoke stress headed N=2, workload 20s: **PASS** — errors=0, goroutine before=3 after=3, orphan assert xanh, không log lỗi Close
- Đo riêng: `Launch → Close` headed = **332ms**, err=nil → budget 2s không bị đốt, không có rủi ro 2s×N khi đóng fleet

## Ghi chú / debt

- Job Object gán SAU `cmd.Start()`: child sinh trong vài µs đầu về lý thuyết có thể ngoài job (CREATE_SUSPENDED không khả thi qua `exec.Cmd`). Fallback taskkill `/T` phủ theo PPID.
- `procExitTimeout` là `var` (không phải const) để test rút ngắn — seam test-only duy nhất thêm vào.
- Z2-7 (lỗi giả khi Firefox tự thoát) mới chỉ được xử lý trong nhánh reap; nhánh khác vẫn thuộc phase sau.
