---
title: "firefoxkit load hardening (leak/race/perf under 200-thread)"
description: ""
status: pending
priority: P1
branch: "master"
tags: []
blockedBy: []
blocks: []
created: "2026-07-21T13:21:49Z"
createdBy: "plan"
source: skill
---

# firefoxkit load hardening (leak/race/perf under 200-thread)

## Overview

Fix phòng ngừa các bug degrade-over-time/leak dưới tải cao (≈200 browser/máy, 50 luồng/browser,
chạy nhiều giờ) mà audit trước (`260719-1620-firefoxkit-leak-fixes`) bỏ sót, + giảm tải latency
move + harness đo thực nghiệm. Backend production = BiDi. Nguồn:
`plans/reports/brainstorm-260721-1950-firefoxkit-load-audit.md` (scope Mức B, user duyệt).

**Nguyên tắc:** mỗi phase TDD — viết test khóa hành vi trước, implement sau, giữ xanh mọi test
stress/concurrency hiện có. YAGNI/KISS: không thêm knob/tối ưu chưa có bằng chứng cần.

## Phases

| Phase | Name | Status |
|-------|------|--------|
| 1 | [F1 Send default timeout](./phase-01-f1-send-default-timeout.md) | Pending |
| 2 | [F2 write-teardown guard](./phase-02-f2-write-teardown-guard.md) | Pending |
| 3 | [L1 non-blocking context prune](./phase-03-l1-non-blocking-context-prune.md) | Pending |
| 4 | [PERF adaptive cursor knob](./phase-04-perf-adaptive-cursor-knob.md) | Completed |
| 5 | [Stress harness empirical](./phase-05-stress-harness-empirical.md) | Completed |

Phase 1–4 độc lập (khác file), có thể làm tuần tự bất kỳ; test dùng mock harness
(`newMockBrowser` / `fakeTransport`, KHÔNG cần Firefox thật). Phase 5 (harness thực nghiệm, build
tag `stress`, gate `FIREFOXKIT_FIREFOX_PATH`) làm cuối để đo hiệu quả P1–P4 + trả lời câu hỏi mở
Firefox-actor serialization.

## Key Decisions

- **F1:** backstop timeout ở tầng `bidi.Client.Send` khi caller ctx **không có deadline** — cấu hình
  được, mặc định rộng (≥ 60s) để không cắt RPC hợp lệ dài nhất (performActions ~1s). Không ép mọi
  caller tự set timeout (tránh sửa rải rác + quên).
- **F2:** tách write-ctx khỏi caller-ctx — `writeJSON` dùng ctx background có timeout riêng để caller
  cancel/timeout KHÔNG giật sập cả WebSocket giữa lúc ghi frame. Response-wait vẫn theo caller ctx.
- **L1:** handler `contextDestroyed` chỉ làm phần gỡ `b.pages` (rẻ, dưới lock) rồi **offload cleanup
  RPC** sang drainer có buffer + vài worker → subscription worker trả ngay → queue-256 không tràn →
  không drop → không leak `b.pages`. KHÔNG goroutine-per-event (tránh anti-pattern L2).
- **PERF:** expose cap step/delay của ProfileHuman (cấu hình) + doc map concurrency→profile. Thành
  thật: knob chỉ đánh đổi human-likeness lấy tốc độ, KHÔNG gỡ được CPU oversubscription gốc.

## Dependencies

- Follow-up của `260719-1620-firefoxkit-leak-fixes` (status: done) — sửa cái nó bỏ sót. Không blocking.
- Không overlap chặn với plan pending khác.

## Validation Log

### Verification Results (session 1)
- Claims checked: file:line then chốt (base ctx, Send callers, bidi.Dial Options, options.go pattern).
- Verified: tất cả | Failed: 0 | Unverified: 0 | Tier: Full (5 phase).
- Ghi chú: `b.ctx()` fallback trả `context.Background()` (browser.go:59) → càng cần F1 backstop.

### Decisions (session 1)
- **F2 semantics:** chốt **mất khả năng caller hủy Write giữa chừng**, dùng writeTimeout ~10s nội bộ.
  Write localhost nhanh, đổi lại không bao giờ giật sập WebSocket. (đúng thiết kế P2)
- **P4:** chốt **thêm `WithCursorTuning`** (không chỉ doc). Default giữ nguyên hành vi. (đúng thiết kế P4)
- **F2 test fidelity:** chốt **CẢ hai** — fakeTransport (go test mặc định) + 1 httptest WS thật
  (tag `wsintegration`). → cập nhật phase-02 Related Files + Success Criteria.

### Whole-Plan Consistency Sweep (session 1)
- Re-read plan.md + 5 phase: không thấy thuật ngữ/quyết định mâu thuẫn. 3 quyết định trên đều CONFIRM
  thiết kế gốc (chỉ F2-test thêm 1 file test). Không có contradiction chưa giải.
