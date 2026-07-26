---
phase: 3
title: "Run 100 headed fleet on this machine"
status: completed
priority: P1
effort: "1.5h"
dependencies: [2]
---

# Phase 3: Run 100 headed fleet on this machine

## Overview

Chạy đúng 1 lần run đắt: `TestStress_FleetWorkload` với N=100, headed, `-race`, binary FirefoxCore_v149,
thu toàn bộ output + số liệu sampler vào plan reports. KHÔNG lặp run này để debug — finding đưa về Phase 4.

## Requirements

- Functional: run hoàn tất (pass hoặc fail đều là dữ liệu); log đầy đủ được lưu; race report (nếu có) giữ nguyên văn.
- Non-functional: cảnh báo user TRƯỚC khi chạy (desktop chiếm dụng ~5-10 phút); không chạy song song việc nặng khác.

## Implementation Steps

1. Pre-flight (PowerShell):
   - Binary tồn tại: `Test-Path 'C:\Users\admin\.multiprofile\resources\browser\gpm_firefox_browser\FirefoxCore_v149\firefox.exe'`
   - RAM khả dụng ≥ 90GB trống (`Get-CimInstance Win32_OperatingSystem | Select FreePhysicalMemory`); disk %TEMP% ≥ 10GB (100 profile).
   - Đếm firefox.exe đang chạy sẵn (nhiễu RSS) — ghi lại con số.
   - **Thông báo user + chờ xác nhận** trước khi bắn 100 cửa sổ (AskUserQuestion — hành động chiếm máy).
2. Run (PowerShell, từ repo root):
   ```powershell
   $env:FIREFOXKIT_FIREFOX_PATH = 'C:\Users\admin\.multiprofile\resources\browser\gpm_firefox_browser\FirefoxCore_v149\firefox.exe'
   $env:FIREFOXKIT_FLEET_N = '100'
   $env:FIREFOXKIT_HEADED = '1'
   $env:FIREFOXKIT_WORKLOAD_SECONDS = '180'
   go test -tags stress -race -run TestStress_FleetWorkload -v -timeout 60m 2>&1 | Tee-Object plans\260726-2019-firefoxkit-leak-race-audit-100-fleet-stress\reports\fleet-100-headed-run-raw.log
   ```
   (Launch stagger 250ms × 100 ≈ 25s; workload 180s; close + settle; tổng dự kiến ~5-8 phút.)
3. Hậu kiểm: không còn firefox.exe mồ côi (`tasklist`), profile temp dirs đã xóa; nếu còn → bản thân đó là finding (ghi id D-x).
4. Trích số liệu sampler thành bảng tóm tắt `reports/fleet-100-headed-run-summary.md`: đường RSS tổng theo thời gian, goroutine trước/trong/sau, heap, error count, race report nguyên văn (nếu có), degradation cảm nhận được (thời gian 1 vòng workload đầu vs cuối).

## Related Code Files

- Create: `plans/260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/fleet-100-headed-run-raw.log`, `fleet-100-headed-run-summary.md`
- Modify: (none — không sửa code trong phase run; nếu harness lỗi vặt → quay lại Phase 2 sửa, smoke lại N=2 rồi mới run lại 100)

## Existing code audit / Reuse strategy

N/A — phase vận hành, không viết code mới.

## Success Criteria

- [ ] Run 100 headed hoàn tất, raw log lưu đủ (kể cả khi test FAIL — fail là dữ liệu)
- [ ] Bảng summary có: RSS timeline, goroutine before/during/after, heap, error count, race verdict
- [ ] Zero firefox.exe mồ côi + zero profile dir sót sau run (hoặc ghi thành finding động)
- [ ] User được hỏi xác nhận trước khi chiếm máy

## Risk Assessment

- RAM cạn giữa run → safety-valve Phase 2 tự abort + đóng fleet; nếu kích hoạt: hạ N (vd 60) và ghi rõ N thực tế vào summary — KHÔNG âm thầm đổi mục tiêu, báo user.
- 100 cửa sổ headed vượt giới hạn GDI/desktop heap Windows → nếu launch fail hàng loạt sau ~N cửa sổ, ghi ngưỡng thực tế, hỏi user chọn: giảm N hoặc chuyển headless cho phần còn lại.
- `-race` làm Go chậm 2-10× → duration 180s vẫn đủ; không tăng thêm để tránh chiếm máy lâu.
