---
phase: 1
title: "Static audit 4 zones parallel"
status: completed
priority: P1
effort: "2h"
dependencies: []
---

# Phase 1: Static audit 4 zones parallel

## Overview

Audit tĩnh leak/race MỚI trên toàn codebase, chia 4 vùng, 4 agent `code-reviewer` chạy **song song**
(1 message, 4 tool call). Đầu ra: 4 report ≤80 dòng + 1 bảng finding tổng hợp. KHÔNG sửa code.

## Context Links

- Brainstorm: `plans/reports/brainstorm-260726-2019-firefoxkit-leak-race-100-fleet-stress.md`
- Audit cũ để đối chiếu (không tin mù): plan `260721-1950-firefoxkit-load-hardening`, plan `260719-1620-firefoxkit-leak-fixes`

## Requirements

- Functional: mỗi vùng có report riêng; finding có `file:line`, loại (leak goroutine / leak memory / race / deadlock / unbounded growth), severity (P1/P2/P3), bằng chứng, đề xuất fix ngắn, confidence.
- Non-functional: report ≤80 dòng/vùng (context-hygiene); agent chỉ đọc, không edit.

## Architecture — phân vùng

| Zone | Files | Lens đặc thù |
|---|---|---|
| Z1 bidi core | `internal/bidi/client.go`, `transport.go`, `events.go`, `session.go`, `script.go`, `network.go`, `message.go` | Re-verify F1 backstop (`client.go:42`), F2 write-ctx (`transport.go:33-93`), pending-call cleanup khi Close, event queue 256 drop path |
| Z2 browser lifecycle | `browser.go`, `browser_context_lifecycle.go`, `browser_spawn.go`, `browser_waitpage.go`, `browser_connect.go`, `browser_launch.go`, `browser_pages.go` | Cleanup drainer (`browser_context_lifecycle.go`), `WaitForNewPage` (commit `d2ec92b` — CHƯA audit), double-Close, profile temp dir |
| Z3 page layer | `page_listeners.go`, `page_console.go`, `page_dialog.go`, `page_downloads_api.go`, `page_headers.go`, `page_eval.go`, `page_action.go` | Handler đăng ký/hủy đối xứng, sub/unsub race với Close, callback goroutine thoát |
| Z4 input (CHƯA TỪNG audit) | `input/cursor_engine.go`, `cursor_path.go`, `cursor_remote.go`, `cursor_tuning.go`, `keyboard.go`, `keyboard_human.go`, `backend_bidi*.go`, `backend_native_windows*.go`, `touch.go` | Ưu tiên cao nhất — code mới nhất; timer/sleep trong path gen, state chia sẻ giữa MoveTo calls, rand nguồn chia sẻ |

## Checklist chung (đưa vào prompt mỗi agent)

Mỗi `go` statement có đường thoát khi Close/ctx cancel; send vào channel đóng/đầy; map/slice truy cập dưới lock (kể cả đọc); ctx propagation không đứt; `time.Timer`/`Ticker` được Stop; handler deregistration đối xứng với registration; double-close an toàn; `sync.WaitGroup` Add/Done cân; buffer/queue có bound; giữ tham chiếu `*Page`/`ElementHandle` sau destroy.

## Implementation Steps

1. Chạy baseline máy móc (main session, không cần agent): `go vet ./...`, `go build ./...`, `go test -race ./... -count=1` (unit suite, KHÔNG tag stress) — ghi kết quả vào report tổng hợp.
2. Spawn 4 agent `code-reviewer` song song (1 message), mỗi agent nhận: danh sách file vùng mình, checklist trên, lens đặc thù, đường dẫn report output, giới hạn 80 dòng. Prompt nêu rõ: "chỉ finding có bằng chứng `file:line`, không khuyến nghị chung chung".
3. Thu 4 report: `plans/260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z{1..4}-{slug}.md`.
4. Main session hợp nhất thành bảng finding sơ bộ (id Z{n}-{k}, chờ Phase 3 bổ sung bằng chứng động) — lưu `reports/static-audit-findings-combined.md`.

## Related Code Files

- Create: `plans/260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z1-bidi-core.md`, `audit-z2-browser-lifecycle.md`, `audit-z3-page-layer.md`, `audit-z4-input-package.md`, `static-audit-findings-combined.md`
- Modify: (none — read-only phase)

## Existing code audit

Reuse-scout không áp dụng cho phase read-only (không viết code mới). Verdict harness nằm ở Phase 2.

## Reuse strategy

N/A — phase chỉ sinh report.

## Success Criteria

- [ ] `go vet` + `go build` + `go test -race` (unit) xanh, kết quả ghi lại
- [ ] 4 report vùng tồn tại, mỗi finding có file:line + severity + confidence
- [ ] Bảng hợp nhất `static-audit-findings-combined.md` tồn tại, finding đánh id ổn định (Z1-1, Z4-2…)
- [ ] Không file code nào bị sửa trong phase này

## Risk Assessment

- Agent trả finding lý thuyết không bằng chứng → prompt ép file:line + confidence; Phase 4 sẽ hạ hạng finding không có bằng chứng động.
- 2 vùng đã audit 2 lần có thể ra ít finding → chấp nhận, đó là tín hiệu tốt; Z4 là trọng tâm.
