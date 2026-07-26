# Run summary — 100 tiến trình Firefox headed, workload 180s, -race

Raw log: `fleet-100-headed-run-raw.log`. Kết quả test: **FAIL** (fail = dữ liệu, đúng thiết kế discovery).
Điều kiện: N=100 headed, CursorFast, BiDi backend, `-race`, binary FirefoxCore_v149, máy 128GB RAM, 0 firefox trước run.

## Số liệu chính

| Mốc | goroutines | HeapAlloc | Sys | allFirefoxRSS | freePhys |
|---|---|---|---|---|---|
| Trước launch | 3 | — | — | 0 | 113.7GB |
| Post-launch (100 browser) | 803 | 8MB | — | 76.3GB | 79.1GB |
| t=19s (đang tải) | 1004 | 13MB | 57MB | 89.7GB | 72.7GB |
| t=8m36s → 19m34s (3 mẫu) | 804 | 13→10MB | 94MB | 89.7GB ổn định | ~65.9GB |
| Post-close + settle | **267** | 6MB | — | (91-103 process CÒN SỐNG) | — |

- Workload: **1414 iterations tổng**, **8 lỗi** — toàn bộ là `firefoxkit: timeout` ở navigate/newpage-navigate trong ~20s đầu (cửa sổ launch-stampede; sau đó 0 lỗi).
- Wall time test: **2334s (~39 phút)** cho 180s workload — xem "hiện tượng sampler" bên dưới.

## Finding động (id D-*)

| ID | Hiện tượng | Bằng chứng | Mức |
|---|---|---|---|
| **D-1** | **Goroutine leak sau khi Close toàn fleet: 3 → 267 (delta 264, ~2.6/browser)** | SUMMARY line, assertion fail delta>20 | **P1 động** — cần attribution stack (run chẩn đoán) |
| **D-2** | **Orphan process: 103 firefox.exe còn sống SAU khi go test thoát**, 100% từ binary test; profile dirs đã bị xóa (0 sót) → Close() xóa profile nhưng không reap process (headed). Smoke N=2 headless không có hiện tượng này. | `Get-Process` sau run: 103 → tự giảm 91 → phải Stop-Process 3 đợt mới về 0 | **P1 động** |
| D-3 | 8 navigate timeout (20s) trong cửa sổ launch-stampede đầu | log lines browser 11/22/33/50/52/59/61/67 | P3 (độ trễ, không phải leak) |
| D-4 | Sampler đói CPU/process-spawn: tick 15s nhưng mẫu cách nhau tới 8 phút — `rssKB` spawn `tasklist` ×100/mẫu + `-race` + máy full tải | timestamps mẫu: 19s, 8m36s, 14m6s, 19m34s | P3 (bài học harness) |
| D-5 | RSS Firefox ổn định 89.7GB suốt tải (~900MB/browser headed); heap Go phẳng 6-13MB; freePhys đáy 65.9GB — safety valve (8GB) không kích hoạt | 3 mẫu giữa run | Info — trả lời "100 luồng chạy nổi": CÓ về RAM |

## Đối chiếu nhanh với audit tĩnh (chốt ở Phase 4)

- D-1 khớp họ finding **Z1-4** (cặp goroutine `wg.Wait` bị bỏ rơi khi drain timeout — dự đoán "fleet 100 → tới 100 cặp" ≈ 200 goroutine) + có thể **Z2-3/Z2-6**. Cần stack dump để chốt.
- D-2 chưa có finding tĩnh tương ứng trực tiếp (Z2-7 là láng giềng — Close trả lỗi giả `os.ErrProcessDone`) → lỗ hổng audit tĩnh, đúng vai trò của mũi động.
- Hậu kiểm: 0 profile dir sót (`firefoxkit-profile-*`) — cleanup profile ĐẠT kể cả khi process leak.

## Ghi chú vận hành

- Desktop chiếm dụng thực tế ~39 phút (dự kiến 6-9) do sampler + `-race` — lần sau: sample bằng 1 lần `tasklist` IMAGENAME thay vì per-PID, hoặc giảm tần suất.
- Orphan đã được dọn thủ công (3 đợt Stop-Process), RAM về 110.9GB, 0 firefox còn lại.
