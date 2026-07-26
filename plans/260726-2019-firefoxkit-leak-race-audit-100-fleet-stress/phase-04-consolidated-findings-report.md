---
phase: 4
title: "Consolidated findings report"
status: completed
priority: P1
effort: "1h"
dependencies: [1, 3]
---

# Phase 4: Consolidated findings report

## Overview

Đối chiếu finding tĩnh (Phase 1) với bằng chứng động (Phase 3), xếp hạng, đề xuất fix. Deliverable
cuối của plan — fix thật (nếu có finding) sẽ là plan riêng sau khi user duyệt danh sách.

## Requirements

- Functional: mỗi finding tĩnh được gắn 1 trong 3 nhãn bằng chứng động: **CONFIRMED** (số liệu/race report khớp), **NOT-OBSERVED** (workload có chạm đường code mà không thấy triệu chứng), **NOT-EXERCISED** (workload không chạm — giữ nguyên hạng tĩnh). Finding động không có finding tĩnh tương ứng (race report bất ngờ, RSS tăng không giải thích, firefox mồ côi) đánh id D-{k}.
- Non-functional: report ≤120 dòng; theo `review-audit-self-decision.md` — không auto-hạ finding đã verified chỉ vì "không thấy trong 1 run 3 phút".

## Implementation Steps

1. Đọc `static-audit-findings-combined.md` + `fleet-100-headed-run-summary.md` + raw log (grep race/error).
2. Ma trận finding × bằng chứng → xếp hạng: P1 = CONFIRMED race/leak; P2 = tĩnh confidence cao + NOT-EXERCISED; P3 = lý thuyết/NOT-OBSERVED.
3. Mỗi finding P1/P2: đề xuất fix 2-4 dòng (hướng, file, trade-off) — KHÔNG implement.
4. Viết `reports/leak-race-consolidated-findings-and-fix-proposals.md`; kèm mục "Câu trả lời cho user": có leak/race không, ở đâu, nghiêm trọng cỡ nào, 100 headed chạy nổi không (số liệu thật).
5. Nếu có ≥1 finding P1 → khuyến nghị plan fix tiếp theo (nêu scope ước lượng); nếu zero → kết luận sạch kèm giới hạn của bằng chứng (duration 180s, workload cụ thể).

## Related Code Files

- Create: `plans/260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/leak-race-consolidated-findings-and-fix-proposals.md`
- Modify: (none)

## Existing code audit / Reuse strategy

N/A — phase tổng hợp báo cáo.

## Success Criteria

- [ ] Mọi finding tĩnh có nhãn CONFIRMED / NOT-OBSERVED / NOT-EXERCISED
- [ ] Finding động không khớp tĩnh được ghi id D-{k}
- [ ] Xếp hạng P1/P2/P3 + đề xuất fix cho P1/P2
- [ ] Mục kết luận trả lời thẳng câu hỏi gốc của user (leak? race? 100 luồng chạy nổi?)

## Risk Assessment

- Cám dỗ "sạch vì không thấy" → nhãn NOT-EXERCISED tồn tại chính để chống kết luận quá tay; ghi rõ giới hạn coverage của workload.
