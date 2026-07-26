---
title: "firefoxkit leak/race full audit + 100-process headed fleet stress"
description: "Audit tĩnh toàn bộ 4 vùng song song + stress harness workload hỗn hợp, chạy 100 tiến trình Firefox headed trên máy dev, đối chiếu finding tĩnh với bằng chứng động"
status: done
priority: P1
branch: "master"
tags: [stress, leak, race, audit]
blockedBy: []
blocks: []
created: "2026-07-26T13:57:04Z"
createdBy: "plan"
source: skill
---

# firefoxkit leak/race full audit + 100-process headed fleet stress

## Overview

Thực thi Phương án C đã duyệt trong brainstorm
[brainstorm-260726-2019-firefoxkit-leak-race-100-fleet-stress.md](../reports/brainstorm-260726-2019-firefoxkit-leak-race-100-fleet-stress.md):
audit tĩnh mới toàn bộ codebase (kể cả vùng chưa từng audit: `input/`, spawn, listeners) **song song**
với việc viết stress harness workload hỗn hợp; sau đó chạy đúng **1 lần** run đắt: 100 tiến trình
Firefox **headed** trên máy này với `-race` + sampling RSS/goroutine; cuối cùng hợp nhất finding
tĩnh × số liệu động thành báo cáo xếp hạng.

- Binary: `C:\Users\admin\.multiprofile\resources\browser\gpm_firefox_browser\FirefoxCore_v149\firefox.exe`
- Máy: Windows 10, 128GB RAM. Headed × 100 ⇒ RAM ước 40-80GB, desktop chiếm dụng trong lúc chạy (user đã chấp nhận).
- Native Windows input backend KHÔNG stress song song được (chỉ điều khiển cửa sổ focus) → workload dùng BiDi input backend. Ngoài scope.

## Phases

| Phase | Name | Status |
|-------|------|--------|
| 1 | [Static audit 4 zones parallel](./phase-01-static-audit-4-zones-parallel.md) | Completed |
| 2 | [Fleet workload stress harness](./phase-02-fleet-workload-stress-harness.md) | Completed |
| 3 | [Run 100 headed fleet on this machine](./phase-03-run-100-headed-fleet-on-this-machine.md) | Completed |
| 4 | [Consolidated findings report](./phase-04-consolidated-findings-report.md) | Completed |

**Thứ tự:** Phase 1 ∥ Phase 2 (độc lập, chạy song song). Phase 3 cần Phase 2. Phase 4 cần Phase 1 + 3.

## Key Decisions (từ brainstorm, user đã chốt)

- **Hình dạng tải:** 100 tiến trình Firefox riêng biệt (khớp production ~50 process/user), KHÔNG phải 100 goroutine/1 browser.
- **Headed** theo yêu cầu user — kết quả CPU/GPU thật hơn; harness vẫn default headless (knob `FIREFOXKIT_HEADED=1`) để không phá test cũ.
- **Audit mới toàn bộ** — không tin mù 2 vòng audit cũ; fix F1/F2/L1 đã vào code (verified: `internal/bidi/client.go:42`, `internal/bidi/transport.go:33`, `browser_context_lifecycle.go:12-18`) nhưng Phase 1 re-verify dưới lens mới.
- **Reuse verdict** (`reuse-scout` report [reuse-scout-260726-2019-fleet-workload-harness.md](../reports/reuse-scout-260726-2019-fleet-workload-harness.md)): REUSE-AS-IS `stressFirefox`/`rssKB`/`settleGoroutines`/`fleetSize`/`latencyStats`/`computeStats`; REUSE-EXTEND `launchFleet` (+param headless); FORK-NEW workload loop + sampler.
- **1 run đắt duy nhất** ở Phase 3; nếu Phase 1 ra finding lớn sau run → run xác nhận thứ 2 là follow-up, không nằm plan này.
- Fix thật cho finding (nếu có) nằm **plan sau** — plan này dừng ở báo cáo xếp hạng + đề xuất fix.

## Dependencies

- Liên quan (không blocking): `260721-1950-firefoxkit-load-hardening` — phases F1/F2/L1 đã implement trong code (commits `f63b764`, transport writeTimeout, cleanup drainer); metadata plan cũ được đồng bộ trong session này. Phase 1 của plan này re-verify các fix đó.
- Test suite `stress` hiện có (`integration_stress_test.go`, `integration_stress_move_test.go`) — nền tảng tái sử dụng, KHÔNG được làm gãy.
