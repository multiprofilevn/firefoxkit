# Postfix verify — headed×100 trước/sau fix

Run: N=100 headed, workload 120s, nav-timeout 30s, `-race`. Log: `fleet-100-headed-postfix-verify.log`.

## So sánh trực diện (cùng kịch bản từng fail 2 lần)

| Chỉ số | Trước fix (run 1 / rerun) | **Sau fix** |
|---|---|---|
| Goroutine sau Close toàn fleet | 267 / 237 (leak 264/234) | **3 → 3 (delta 0)** ✅ |
| Firefox orphan sau test | 103 / 125 (phải force-kill 3 đợt) | **0 — tự sạch, không cần kill tay** ✅ |
| Heap Go post-close | 6MB | 5MB ✅ |
| Race detector | sạch | sạch ✅ |
| Close trả lỗi | nil giả ×100 | 2 lỗi THẬT "survived kill" (browser 48, 76) — process chết ngay sau đó (đếm cuối = 0) |
| Workload errors | 8 (nav timeout 20s) | **2** (nav timeout 30s, browser 13 & 33, cửa sổ stampede) |
| Kết quả test | FAIL (leak + orphan) | FAIL **chỉ vì** 2 nav-timeout (assert errors==0) |

## Kết luận

- **D-1 (goroutine leak) và D-2 (orphan process): ĐÃ CHẾT** — mục tiêu của plan đạt, máy tự sạch 100%.
- 2 tồn dư nhỏ, đều KHÔNG phải leak:
  1. **2/956 navigate timeout (0.2%)** trong cửa sổ launch-stampede headed×100 — hiện tượng oversubscription (D-3 đã ghi nhận từ run đầu, giảm từ 8 → 2 khi nới 20s → 30s). Bản chất perf, không phải bug shutdown.
  2. **2 báo lỗi "survived kill" eager**: verify-dead 5s hết hạn dưới close-storm trong khi process thực tế chết ngay sau — lỗi trung thực nhưng hơi vội; backlog P3: nới `procExitTimeout` hoặc re-check aliveness trước khi báo lỗi.

## Số liệu vận hành

- Post-launch: 803 goroutine, RSS fleet 76.3GB; đỉnh 88.4GB; freePhys đáy 66.7GB — headless lẫn headed đều trong khả năng máy 128GB.
- Tổng thời gian run 1042s (~17.4 phút), phần lớn là close tuần tự 100 browser dưới `-race`.
