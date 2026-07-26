---
phase: 2
title: "Cleanup drainer drop-on-saturation"
status: completed
priority: P2
effort: "2h"
dependencies: []
---

# Phase 2: Cleanup drainer drop-on-saturation

## Overview

Fix Z2-1 + Z2-3 (audit Z2). Khi `cleanupCh` đầy, code hiện tại chạy cleanup RPC **inline trên
subscription worker** → chặn tới ~30s/event → tràn queue event 256 → drop `contextDestroyed` →
leak `b.pages`. **User đã duyệt: DROP thay vì chạy inline** (page đã detach; `contextDestroyed`
nghĩa là context chết phía Firefox, server-side state chết theo — RPC chỉ là belt-and-suspenders).

## Requirements

- Functional:
  1. Nhánh saturated ([browser_context_lifecycle.go:64-73](../../browser_context_lifecycle.go)): bỏ `target.cleanup(cctx)` inline → tăng counter drop (atomic) + `Warnf` một dòng có contextID. Page ĐÃ rời `b.pages` trước đó nên không leak map.
  2. Z2-3 — thứ tự trong `Browser.Close`: `signalCleanupDrainer()` TRƯỚC `baseCancel()` để graceful drain deterministic (hiện chọn ngẫu nhiên giữa stop và work dưới ctx đã cancel). Worker cleanup ctx dùng `Background+timeout` nhất quán với `drainRemainingCleanup` (đã vậy ở `:143`).
  3. Expose counter drop qua log khi Close (tổng số drop trong đời browser) — không cần API public.
- Non-functional: không đổi chữ ký public; giữ xanh `browser_context_lifecycle_load_test.go` + `lowprio_cleanup_test.go`.

## Related Code Files

- Modify: `browser_context_lifecycle.go`, `browser.go` (thứ tự signal trong Close)
- Modify (test): `browser_context_lifecycle_load_test.go` — thêm case saturation

## Existing code audit

Audit Z2 (`../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z2-browser-lifecycle.md`) đã map toàn bộ vùng này (Z2-1/Z2-3/Z2-6) — thay vai trò reuse-scout. Verdict: **REUSE-EXTEND** cơ chế drainer hiện có (đổi hành vi nhánh fallback), không viết cơ chế mới.

## Reuse strategy

Sửa nhánh fallback trong hàm hiện có; test extend file load-test hiện có (mock harness `newMockBrowser`, không cần Firefox thật). Trả lời luôn câu hỏi mở Z2 #2 của audit: "được phép bỏ cleanup khi saturated" — CÓ, user duyệt 2026-07-26.

## Implementation Steps (TDD)

1. **Test trước**: case flood > `cleanupChCap` (512) events — assert: subscription worker không bị chặn (event sau flood được xử lý trong ~ms), counter drop > 0, `b.pages` không còn entry của context đã destroy.
2. Test thứ tự Close: mock peer treo → Close vẫn bounded, drain worker thoát deterministic (chạy lặp 50 lần không flaky).
3. Implement drop + counter; swap thứ tự signal/cancel; chạy `go test -race ./...`.

## Success Criteria

- [x] Test saturation mới xanh, chạy lặp không flaky (`-race -count=5`)
- [x] Không còn đường code gọi `cleanup` trên subscription worker
- [x] `go test -race ./...` xanh; `go vet/build -tags stress` xanh (stress fleet chạy ở phase 6)

## Risk Assessment

- Test cũ `browser_context_lifecycle_load_test.go:113` dựa vào base-ctx cancel để join nhanh — nếu đổi thứ tự làm chậm join, điều chỉnh test theo hành vi mới (có chủ đích, ghi rõ trong commit).
