---
phase: 1
title: "Process-tree reap and verify-dead in Close"
status: completed
priority: P1
effort: "4h"
dependencies: []
---

# Phase 1: Process-tree reap and verify-dead in Close

## Overview

Fix root cause D-2 (kéo theo hết D-1): `Browser.Close` launch-path phải đảm bảo **toàn bộ process
tree Firefox chết thật** trước khi trả về, và trả lỗi thật khi không giết được. Bằng chứng: re-run
headed×100 — 78/100 browser để lại process sống dù `proc.Kill()` trả nil; 3 goroutine/browser kẹt
trên pipe/Wait của process chưa chết (stack dump trong
`../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/fleet-100-headed-diagnostic-rerun.log`).

## Requirements

- Functional:
  1. Gửi BiDi `browser.close` graceful TRƯỚC khi kill (lệnh chuẩn BiDi module `browser`; thêm wrapper nhỏ vào `internal/bidi` nếu chưa có), budget ngắn (~2s), lỗi bỏ qua.
  2. **Windows (production): Job Object** — tạo job với `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` lúc spawn, `AssignProcessToJobObject` ngay sau Start; Close chỉ cần `CloseHandle(job)` → cả tree chết. Fallback khi assign fail (đã ở trong job khác…): `taskkill /T /F /PID`.
  3. **Verify-dead:** sau kill, chờ process exit tối đa ~5s qua channel done của goroutine `cmd.Wait` (KHÔNG gọi Wait 2 lần — xử lý luôn Z2-8 double-Wait: `Launch.func2` là chủ sở hữu duy nhất của `cmd.Wait`, Close chờ done channel); hết hạn → trả lỗi thật `firefoxkit: firefox pid=... survived kill`.
  4. Non-Windows: giữ hành vi hiện tại + `Setpgid`/kill process group nếu rẻ; không bắt buộc trong phase này (production = Windows).
- Non-functional: `Close()` vẫn bounded (~≤ graceful + 5s); double-Close an toàn như cũ; không đổi chữ ký public.

## Related Code Files

- Modify: `browser.go` (Close), `browser_launch.go` (spawn + job object + done channel), `internal/bidi/session.go` hoặc file mới nhỏ `internal/bidi/browser_close.go` (lệnh `browser.close`)
- Create: `browser_process_tree_windows.go` (job object helpers, build tag windows), `browser_process_tree_windows_test.go`

## Existing code audit

Vùng này vừa được audit bởi agent Z2 (report `../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z2-browser-lifecycle.md`) — Z2-7 (lỗi giả khi Firefox tự thoát), Z2-8 (double-Wait), Z2-9 (scanner goroutine) đều nằm trong file sẽ sửa; audit này thay vai trò reuse-scout cho phase. Không có helper process-tree nào tồn tại trong repo → verdict **FORK-NEW** cho job-object helper; **REUSE-EXTEND** `Launch.func2` (thêm done channel).

## Reuse strategy

FORK-NEW `browser_process_tree_windows.go` (syscall thuần, không dep mới); REUSE-EXTEND cấu trúc Launch hiện có thay vì viết launcher mới. Lệnh `browser.close` theo đúng pattern command wrapper sẵn có trong `internal/bidi` (marshalling thuần).

## Implementation Steps (TDD)

1. **Test trước** — `browser_process_tree_windows_test.go`: spawn `cmd.exe /c "ping -n 30 127.0.0.1 & ping -n 30 127.0.0.1"`-style tree (hoặc powershell spawn con), gọi helper reap, assert cả cha lẫn con chết ≤2s. Test verify-dead: process đã chết trước → helper trả nil ngay.
2. Implement job-object helper: `newProcessJob() → assign(pid) → closeKill()`; wire vào `browser_launch.go` sau `cmd.Start()`.
3. Thêm done channel cho `Launch.func2` (`cmd.Wait` single-owner); Close chờ channel với timeout 5s thay vì `b.proc.Wait()`.
4. Thêm BiDi `browser.close` wrapper + gọi đầu Close (budget 2s, best-effort).
5. `go test -race ./...` + smoke stress N=2 **headed** (`FIREFOXKIT_HEADED=1`) — orphan assert phải PASS.

## Success Criteria

- [ ] Unit test process-tree reap xanh (Windows)
- [ ] `go test -race ./...` toàn repo xanh; `go vet`/build tags stress xanh
- [ ] Smoke N=2 headed: 0 orphan, goroutine delta ≤ 20, Close không trả lỗi giả
- [ ] Close trả lỗi thật nếu process sống sót (test giả lập bằng process không kill được — nếu khó, chấp nhận unit test nhánh timeout bằng fake done channel)

## Risk Assessment

- Job Object có thể fail khi go test process đã nằm trong job có `JOB_OBJECT_LIMIT_BREAKAWAY` khác → luôn có fallback `taskkill /T /F`; log đường nào được dùng.
- `browser.close` trên peer nghẽn có thể ăn hết 2s budget ×100 browser tuần tự khi đóng fleet → budget ngắn + không retry.
