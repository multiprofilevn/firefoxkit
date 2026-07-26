---
phase: 3
title: "SetExtraHeaders install-remove race fix"
status: completed
priority: P2
effort: "2h"
dependencies: []
---

# Phase 3: SetExtraHeaders install-remove race fix

## Overview

Fix Z3-1 (P1 tĩnh duy nhất) + Z3-2 cùng file (audit Z3). Z3-1: `SetExtraHeaders(A)` đua
`SetExtraHeaders(nil)` → commit map dưới `hi.mu` nhưng install/remove rẽ nhánh theo **tham số**
dưới `installMu` → interleave làm interceptor bị pin vĩnh viễn với headers rỗng. Z3-2: remove
không `UnsubscribeEvents` + `trackServerEvent` append trùng → WS traffic không ai tiêu thụ.

## Requirements

- Functional:
  1. Z3-1: commit `hi.headers` BÊN TRONG `installMu`; nhánh install/remove quyết theo `len(hi.headers)` sau commit ([page_headers.go:44-65](../../page_headers.go)) — trạng thái intercept luôn khớp trạng thái headers đã commit, mọi interleave.
  2. Z3-2: `removeHeaderInterceptor` gọi `UnsubscribeEvents` cho local sub + gỡ tên khỏi `p.serverEvents`; `trackServerEvent` dedupe ([page.go:87-91](../../page.go)).
- Non-functional: hành vi public không đổi (set → áp; nil/empty → gỡ); giữ xanh `TestSetExtraHeaders_ConcurrentInstall_SingleIntercept`.

## Related Code Files

- Modify: `page_headers.go`, `page.go` (trackServerEvent dedupe)
- Modify (test): `concurrency_hardening_test.go` — thêm 2 case

## Existing code audit

Audit Z3 (`../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z3-page-layer.md`) map chính xác 2 finding + chỉ ra gap test (case non-empty vs nil chưa cover) — thay vai trò reuse-scout. Verdict: **REUSE-EXTEND** cơ chế interceptor hiện có, sửa thứ tự commit.

## Reuse strategy

Sửa trong hàm hiện có, không thêm abstraction. Test theo pattern race test sẵn có trong `concurrency_hardening_test.go` (mock BiDi server).

## Implementation Steps (TDD)

1. **Test trước** (fail trên code hiện tại): goroutine A lặp `SetExtraHeaders(map)`, goroutine B lặp `SetExtraHeaders(nil)`, kết thúc bằng `SetExtraHeaders(nil)` → assert trạng thái cuối: intercept ĐÃ GỠ (mock server đếm addIntercept == removeIntercept) và headers rỗng. Chạy `-race`.
2. Test chu kỳ install/remove ×N → assert đúng N unsubscribe + `p.serverEvents` không phình.
3. Implement 2 fix; `go test -race ./...`.

## Success Criteria

- [x] Test race mới xanh và FAIL được trên code cũ (đã xác nhận trước khi fix)
- [x] addIntercept/removeIntercept cân bằng trong mọi test
- [x] `go test -race ./...` xanh

## Risk Assessment

- Giữ `hi.mu` cho reader path của handler (`:92`) — không đổi granularity đọc, chỉ dời chỗ commit; deadlock check: `installMu` → `hi.mu` một chiều duy nhất.
