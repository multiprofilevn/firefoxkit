---
title: "firefoxkit shutdown process-tree reap + P2 hardening"
description: "Fix root cause D-2 (Close không reap process tree → orphan + goroutine leak) + cụm P2 đã thống nhất; verify lại headed×100"
status: done
priority: P1
branch: "master"
tags: [fix, leak, race, shutdown, input]
blockedBy: []
blocks: []
created: "2026-07-26T16:51:06Z"
createdBy: "plan"
source: skill
---

# firefoxkit shutdown process-tree reap + P2 hardening

## Overview

Thực thi cụm fix đã thống nhất với user từ audit `260726-2019` (báo cáo hợp nhất:
[leak-race-consolidated-findings-and-fix-proposals.md](../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/leak-race-consolidated-findings-and-fix-proposals.md)).
Root cause DUY NHẤT của cả 2 P1 động: `Browser.Close` không reap được process tree Firefox
(78/100 headed sống sót dù `Kill()` trả nil) → 3 goroutine/browser kẹt trên pipe/Wait.
Stack dump đã bác bỏ giả thuyết Z1-4 (bidi goroutine thoát sạch).

**Nguyên tắc:** mỗi phase TDD (test khóa hành vi trước, implement sau), giữ xanh toàn bộ unit
`-race` + stress suite. YAGNI: chỉ fix những gì có bằng chứng/quyết định user.

## Phases

| Phase | Name | Status |
|-------|------|--------|
| 1 | [Process-tree reap and verify-dead in Close](./phase-01-process-tree-reap-and-verify-dead-in-close.md) | Completed |
| 2 | [Cleanup drainer drop-on-saturation](./phase-02-cleanup-drainer-drop-on-saturation.md) | Completed |
| 3 | [SetExtraHeaders install-remove race fix](./phase-03-setextraheaders-install-remove-race-fix.md) | Completed |
| 4 | [Write timeout knob](./phase-04-write-timeout-knob.md) | Completed |
| 5 | [Native gesture lock and scale-factor pref](./phase-05-native-gesture-lock-and-scale-factor-pref.md) | Completed |
| 6 | [Verify fleet 100 headed clean](./phase-06-verify-fleet-100-headed-clean.md) | Completed |

Phase 1-5 độc lập file, làm tuần tự 1→5 (1 là P1, còn lại P2). Phase 6 chạy cuối, cần 1-5 xong.

## Key Decisions (user đã chốt / bằng chứng đã ép)

- **Drop cleanup RPC khi drainer bão hòa** — user duyệt ("ok theo ý bạn"). Drop + counter, không chạy inline trên subscription worker nữa.
- **Native backend production tối đa 9 browser đồng thời** (chuột thật) → gesture lock fleet-wide là yêu cầu thật; kèm sắp xếp cửa sổ + scale (đã có `WithNativeWindow`/`SetWindowBounds`; bổ sung pref scale-factor).
- **Z1-4 KHÔNG sửa** — stack dump chứng minh vô tội; để backlog P3.
- **Re-run verify headed×100** — user ủy quyền, Claude quyết CÓ (phase 6; hỏi trước khi chiếm máy).
- Fix theo thứ tự bằng chứng: P1 process reap trước, P2 sau, KHÔNG gộp các P2/P3 chưa thống nhất (Z1-2, Z1-3, Z2-2, Z2-4, Z3-3… ở backlog).

## Dependencies

- Nguồn bằng chứng: plan `260726-2019-firefoxkit-leak-race-audit-100-fleet-stress` (done) — 4 audit vùng + 4 run stress + stack dump.
- Harness verify có sẵn: `integration_stress_fleet_workload_test.go` (orphan assert + stack dump + safety valve).
- Không overlap plan pending khác (`260721-1950` đã completed).
