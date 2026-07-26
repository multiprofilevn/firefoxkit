---
phase: 6
title: "Verify fleet 100 headed clean"
status: completed
priority: P1
effort: "1h"
dependencies: [1, 2, 3, 4, 5]
---

# Phase 6: Verify fleet 100 headed clean

## Overview

Phase verify cuối (user ủy quyền, Claude quyết CÓ): chạy lại đúng kịch bản đã fail —
`TestStress_FleetWorkload` headed×100 — kỳ vọng **PASS**: 0 orphan, goroutine delta ≤ 20. Harness
đã có sẵn orphan assert + stack dump + safety valve từ plan audit.

## Requirements

- Functional: run headed×100 PASS; nếu FAIL → stack dump/close-error log là input cho vòng fix tiếp, KHÔNG patch mù.
- Non-functional: hỏi user trước khi chiếm máy (~10-20 phút); dọn máy sạch sau run (0 firefox, RAM trả lại).

## Implementation Steps

1. Thêm knob `FIREFOXKIT_NAV_TIMEOUT_SECONDS` (default 20) vào harness — run verify dùng 30s để loại nhiễu launch-stampede (8 navigate timeout từng thấy ở cả 2 run 100-headed; là độ trễ oversubscription, không phải bug đang fix). Đổi ~10 dòng test, compile gate + smoke N=2.
2. Chạy full unit: `go vet ./... && go build ./... && go test -race ./... -count=1` — xanh.
3. Smoke N=2 headed + N=2 headless (`-race`) — orphan assert xanh cả hai.
4. **Hỏi user** (AskUserQuestion — chiếm máy) rồi run:
   ```
   FIREFOXKIT_FLEET_N=100 FIREFOXKIT_HEADED=1 FIREFOXKIT_WORKLOAD_SECONDS=120 FIREFOXKIT_NAV_TIMEOUT_SECONDS=30 \
   go test -tags stress -race -run TestStress_FleetWorkload -v -timeout 45m | tee <plan>/reports/fleet-100-headed-postfix-verify.log
   ```
5. Hậu kiểm: `Get-Process firefox` = 0, profile dirs = 0; ghi summary so sánh trước/sau (267→? goroutine, 103→? orphan) vào `reports/postfix-verify-summary.md`.

## Related Code Files

- Modify: `integration_stress_fleet_workload_test.go` (nav-timeout knob)
- Create: `<plan>/reports/fleet-100-headed-postfix-verify.log`, `reports/postfix-verify-summary.md`

## Existing code audit / Reuse strategy

Reuse toàn bộ harness plan audit (orphan assert, stack dump, sampler 1-tasklist). Chỉ thêm 1 knob env theo pattern `workloadDuration` sẵn có.

## Success Criteria

- [x] Goroutine delta = 0 (3→3, trước fix: leak 264/234) + 0 orphan tự sạch (trước: 103/125) — **D-1/D-2 đã chết**
- [x] Máy sạch sau run (0 firefox.exe, không cần kill tay, RAM về nền)
- [x] Summary trước/sau: `reports/postfix-verify-summary.md`
- [x] ~~0 workload error~~ — 2/956 navigate timeout (0.2%, stampede window): **user chấp nhận** là đặc tính perf oversubscription (không phải leak), backlog cùng "survived kill" eager (nới `procExitTimeout`)

## Risk Assessment

- Nếu vẫn còn orphan lẻ tẻ (<78) → fix chưa đủ, có cơ chế thứ 2; stack dump + close-error log định hướng vòng sau — không hạ ngưỡng assert để "cho qua".
