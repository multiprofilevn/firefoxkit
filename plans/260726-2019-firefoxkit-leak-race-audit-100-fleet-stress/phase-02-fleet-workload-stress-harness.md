---
phase: 2
title: "Fleet workload stress harness"
status: completed
priority: P1
effort: "3h"
dependencies: []
---

# Phase 2: Fleet workload stress harness

## Overview

Viết test stress mới `TestStress_FleetWorkload` (file mới, build tag `stress`): N tiến trình Firefox,
mỗi browser chạy vòng lặp workload hỗn hợp (navigate/eval/click/type/tab churn/screenshot), sampler
định kỳ ghi goroutine + Go heap + tổng RSS Firefox, safety-valve RAM. Chạy song song với Phase 1.

## Context Links

- Reuse-scout verdict: `plans/reports/reuse-scout-260726-2019-fleet-workload-harness.md`
- Harness hiện có: `integration_stress_test.go`, `integration_stress_move_test.go`

## Requirements

- Functional:
  - Knob env: `FIREFOXKIT_FLEET_N` (có sẵn, default 25), `FIREFOXKIT_HEADED=1` (mới, default headless), `FIREFOXKIT_WORKLOAD_SECONDS` (mới, default 180).
  - Workload mỗi browser (1 goroutine/browser, lặp đến hết duration): navigate `data:` URL xoay vòng (3 trang inline có button + input) → `Evaluate` JS nhỏ → Click button → Type vào input (BiDi backend, KHÔNG native) → mỗi vòng thứ 5: mở tab mới, navigate, đóng → screenshot xác suất ~1/10.
  - Sampler goroutine mỗi 15s: elapsed, `runtime.NumGoroutine()`, `HeapAlloc`/`Sys` từ `runtime.ReadMemStats`, tổng RSS mọi PID firefox của fleet (reuse `rssKB`), log 1 dòng dạng bảng.
  - Safety-valve: sampler đọc available physical memory (Windows: `GlobalMemoryStatusEx` qua syscall hoặc parse `wmic OS get FreePhysicalMemory`); nếu < 8GB → fail loud + đóng cả fleet ngay.
  - Stagger launch 250ms/instance (headed nặng hơn 50ms hiện tại của MoveTo fleet).
- Non-functional: KHÔNG phá test cũ; `-race` sạch; file mới < ~300 dòng (test file, được miễn ngưỡng 200 LOC nhưng giữ gọn); mọi error workload abort loud, không nuốt.

## Architecture

Assertion cứng sau khi hết duration + Close toàn fleet:
1. Workload error count == 0.
2. `settleGoroutines()` delta so với trước launch ≤ 20.
3. Go `HeapAlloc` sau close không vượt 2× mẫu đầu tiên (bound thô, đủ bắt leak tuyến tính).
4. RSS Firefox: CHỈ log (discovery run đầu, không hard-assert).

PID mỗi browser: lấy qua API hiện có của `Browser` (cook verify tên chính xác — `b.PID()` hoặc tương đương đã được `TestStress_ProfileDir_NoLeak`/`rssKB` call site dùng; grep trước khi viết).

## Related Code Files

- Create: `integration_stress_fleet_workload_test.go`
- Modify: `integration_stress_move_test.go` — `launchFleet` thêm param `headless bool` (line ~376 đổi `WithHeadless(headless)`), cập nhật 2 caller hiện có (line ~405, ~437) truyền `true`.
- Delete: (none)

## Existing code audit

Reuse-scout (haiku) đã chạy — report `plans/reports/reuse-scout-260726-2019-fleet-workload-harness.md`. Verdict: **REUSE-AS-IS** `stressFirefox`, `rssKB`, `settleGoroutines`, `fleetSize`, `latencyStats`, `computeStats`; **REUSE-EXTEND** `launchFleet` (hardcode `WithHeadless(true)` tại line 376); **FORK-NEW** workload loop + periodic sampler (không có prior art gộp workload + sampling).

## Reuse strategy

REUSE-EXTEND `launchFleet` bằng 1 param bool (backward-compatible, sửa 2 caller trong cùng file) thay vì fork helper mới — một nguồn sự thật cho việc launch fleet. Workload loop + sampler là FORK-NEW vì `runFleetMoves` chỉ đo MoveTo và `TestStress_ChurnGrowth_NoLeak` chỉ churn tab; gộp chúng không khả thi mà không viết mới.

## Implementation Steps

1. Sửa `launchFleet` (+param `headless`), cập nhật 2 caller → `go build -tags stress ./...` xanh.
2. Viết `integration_stress_fleet_workload_test.go`: knob env → helper `headedFromEnv()`, `workloadSeconds()`; 3 trang `data:` URL hằng số; hàm `runWorkloadLoop(t, b, dur, errCh)`; sampler + safety-valve; assertion như Architecture.
3. Compile gate: `go vet -tags stress ./...` + `go build -tags stress ./...`.
4. **Smoke thu nhỏ** (bắt buộc trước Phase 3): `FIREFOXKIT_FLEET_N=2`, headless, `FIREFOXKIT_WORKLOAD_SECONDS=20`, `-race` — phải xanh, sampler in đúng, close sạch.
5. Chạy lại test stress cũ smoke nhanh (`-run Fleet_MoveLatency` với N=2) xác nhận không phá caller cũ.

## Success Criteria

- [ ] `go vet`/`go build` với tag `stress` xanh
- [ ] Smoke N=2 headless 20s pass với `-race`, log sampler đúng format
- [ ] Test cũ `TestStress_Fleet_MoveLatency` N=2 vẫn pass (launchFleet extension không phá)
- [ ] Safety-valve RAM có test đường kích hoạt (mock ngưỡng cao) hoặc tối thiểu verify log path bằng tay
- [ ] Error workload không bị nuốt (t.Errorf + abort)

## Risk Assessment

- `data:` URL bị Firefox chặn top-level navigation → fallback: serve trang từ `httptest.Server` local (cook quyết theo thực tế, ghi vào report).
- Screenshot 100 browser cùng lúc gây spike RAM encode → xác suất 1/10 + kích thước viewport nhỏ là đủ giảm.
- API lấy PID không tồn tại public → fallback đọc PID từ `Launch` internals/os.Process nếu exposed; nếu không, tổng RSS lấy qua `tasklist /FI "IMAGENAME eq firefox.exe"` (lọc theo thời điểm start — chấp nhận nhiễu nếu máy có firefox khác; ghi chú trong report).
