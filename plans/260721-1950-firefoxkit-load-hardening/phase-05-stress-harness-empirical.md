---
phase: 5
title: "Stress harness empirical"
status: pending
priority: P2
effort: "5h"
test-strategy: tdd
dependencies: [1, 2, 3, 4]
---

# Phase 5: Stress harness thực nghiệm (đo latency move + growth + Firefox-actor serialization)

## Overview
Bổ sung harness `-tags stress` (Firefox thật) đo những thứ harness cũ chưa có: latency move dưới N
browser/page đồng thời, tăng trưởng goroutine/mem/page qua churn kéo dài, và trả lời câu hỏi mở —
Firefox có serialize `performActions` giữa các browsingContext không. Xác nhận P1–P4 có hiệu quả.

## Key Insights
- Harness cũ (`integration_stress_test.go`, tag `stress`, gate `FIREFOXKIT_FIREFOX_PATH`) chỉ test
  leak goroutine/handle/tab-churn/console-flood — KHÔNG đo latency move.
- Câu hỏi mở quyết định lời khuyên kiến trúc "50 luồng/1 page" (bị mutex nối tiếp) vs "50 page riêng"
  (chỉ bị Firefox input-actor nếu nó serialize). Chỉ đo thực nghiệm mới chốt được.
- Ngưỡng phải tương đối + warmup (theo convention file cũ) để chịu nhiễu máy; skip khi thiếu env.

## Requirements
- Functional: đo p50/p95/max latency của `MouseAPI.MoveTo`/`Click` dưới:
  (a) 1 page, K goroutine đồng thời (kỳ vọng: nối tiếp do mutex per-cursor).
  (b) K page riêng trong 1 browser, mỗi page 1 move đồng thời (đo Firefox-actor có serialize không).
  (c) profile Human vs Fast vs Machine — bảng số liệu so sánh.
- Functional: đo goroutine + RSS + `len(b.pages)` trước/sau vòng churn dài (mở/đóng tab + move lặp)
  → khẳng định P1/P3 không leak (delta ~0 sau settle).
- Non-functional: build tag `stress`; gate env; ngưỡng tương đối; timeout rộng; không vào `go test`
  mặc định.

## Architecture / Cách đo
- Dùng `stressFirefox(t)` + `settleGoroutines()` sẵn có. Thêm helper đo latency: bọc quanh MoveTo,
  gom `[]time.Duration`, tính p50/p95/max.
- Test (b) serialization probe: mở K page, đồng thời gọi move mỗi page, đo tổng wall-clock. Nếu ≈
  1×move → song song; nếu ≈ K×move → Firefox serialize actor. Log kết luận rõ ràng cho user.
- Test growth: vòng lặp T lần {mở tab → move → đóng tab}; đo goroutine/RSS/len(pages) mỗi mốc; assert
  không tăng tuyến tính (so baseline sau warmup).
- Tận dụng P4 tuning để chạy nhanh khi cần (giảm MaxSteps trong test đo growth để rút thời gian).

## Related Code Files
- Create: `integration_stress_move_test.go` (build tag `stress`) — latency + serialization probe.
- Modify (nếu cần helper chung): `integration_stress_test.go` (thêm latency helper, giữ test cũ).

## Test Spec (đây LÀ deliverable của phase — harness đo)
1. `TestStress_MoveLatency_ByProfile`: bảng p50/p95/max cho Human/Fast/Machine (1 page, tuần tự) →
   log; assert Machine < Fast < Human theo mức kỳ vọng (ngưỡng tương đối, rộng).
2. `TestStress_MoveLatency_SamePage_KGoroutines`: K=20 goroutine move trên 1 page; assert tổng thời
   gian ≈ K × single (mutex nối tiếp) — khóa hiểu biết P2.
3. `TestStress_MoveLatency_KPages_Concurrent` (serialization probe): K=20 page, move đồng thời; LOG
   tỉ số wall-clock/single-move + in kết luận "Firefox performActions: parallel|serialized across
   contexts". Không assert cứng (đo khám phá) — nhưng ghi kết quả vào report.
4. `TestStress_ChurnGrowth_NoLeak`: T=500 vòng churn+move; assert delta goroutine ≤ ngưỡng nhỏ,
   `len(b.pages)` về 1, RSS không tăng tuyến tính.
5. Chạy kèm `-race` ở quy mô nhỏ để bắt race mới.

## Implementation Steps
1. Viết helper đo latency + skeleton test theo Test Spec.
2. Chạy trên máy có Firefox thật (`FIREFOXKIT_FIREFOX_PATH=... go test -tags stress -run StressMove
   -v -timeout 20m`); thu số liệu.
3. Ghi kết quả serialization probe + bảng latency vào
   `plans/reports/audit-260721-1950-firefoxkit-load-audit.md` (mục "Kết quả thực nghiệm") + cập nhật
   khuyến nghị profile trong README (P4) nếu số liệu lệch giả định.
4. Nếu probe cho thấy Firefox serialize actor → ghi rõ khuyến nghị: giảm số page/browser hoặc số
   browser/máy thay vì tăng luồng/page.

## Success Criteria
- [ ] Harness chạy được với Firefox thật, skip sạch khi thiếu env.
- [ ] Có bảng latency Human/Fast/Machine + số liệu đồng thời.
- [ ] Trả lời được câu hỏi Firefox-actor serialization (parallel hay serialize) — ghi vào report.
- [ ] Test growth khẳng định P1/P3 không leak (delta ~0 sau settle).

## Risk Assessment
- Phụ thuộc máy có Firefox 129+ + CPU rảnh để số liệu sạch; ngưỡng tương đối tránh flaky.
- Probe (b) có thể bị nhiễu bởi navigation/layout — cô lập bằng trang tĩnh về:blank + toạ độ cố định.
- 200 browser thật khó dựng trong CI → test dùng K nhỏ (10–20) đại diện, ngoại suy có ghi chú
  (no-silent-cap: log rõ K dùng vs quy mô production 200).

## Next Steps
Kết quả feed lại README (P4) + memory. Kết thúc plan.
