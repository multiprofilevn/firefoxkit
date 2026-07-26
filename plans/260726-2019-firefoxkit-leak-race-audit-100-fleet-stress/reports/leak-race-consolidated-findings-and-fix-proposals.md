# Báo cáo hợp nhất — leak/race firefoxkit: audit tĩnh × bằng chứng động

Nguồn: `static-audit-findings-combined.md` (36 finding tĩnh) × 4 run động (`-race` tất cả):

| Run | Kết quả | Bằng chứng chính |
|---|---|---|
| N=2 headless 20s (smoke) | PASS | goroutine 3→3, heap phẳng |
| N=100 **headed** 180s | **FAIL** | **leak 264 goroutine sau Close; 103 firefox orphan; 8 nav timeout lúc stampede** |
| N=10 headed 30s | PASS | 0 lỗi, 3→3, 0 orphan |
| N=100 headless 120s | PASS | 3018 iters, 0 lỗi, 3→3, 0 orphan, RSS ~70GB ổn định |

**Race detector: SẠCH qua cả 4 run.** Go heap: sạch mọi run (4-16MB).

## Trả lời thẳng câu hỏi gốc

1. **Có race condition không?** Không phát hiện race NÀO bằng `-race` qua ~4.700 workload iterations. Tĩnh còn 1 P1 lý thuyết chưa bị kích (Z3-1) + vài race hẹp (Z4-2/Z4-4) ở API workload không dùng.
2. **Có memory leak không?** Go-side: KHÔNG (heap phẳng, goroutine về 3 ở headless mọi N). Nhưng **headed × N=100 có bệnh shutdown thật**: D-1 + D-2 dưới đây.
3. **100 luồng Firefox chạy nổi trên máy này không?** **Headless: nổi, thoải mái** — RSS ~70GB/128GB, 0 lỗi, throughput 3018 iters/120s. **Headed: chạy được khi tải (RSS 89.7GB) nhưng shutdown hỏng** — phải fix trước khi dùng headed×100 production.

## Finding động (CONFIRMED)

| ID | Sev | Hiện tượng | Cơ chế khả dĩ (root-cause candidates) |
|---|---|---|---|
| **D-1** | **P1** | Đóng 100 browser headed → 264 goroutine không thoát (≈2.6/browser). Không tái hiện N=10 headed / N=100 headless. | Headed×100 bão hòa CPU/GDI → shutdown vượt budget: `events.go:143-151` drain-timeout 5s bỏ rơi cặp goroutine `wg.Wait` **vĩnh viễn** (= finding tĩnh **Z1-4**, dự đoán "100 client → tới 100 cặp" khớp bậc độ lớn) + `gracefulCloseBudget` expiry cascade ([browser.go:160-173](../../browser.go)). Attribution stack chính xác cần re-run headed với dump (harness ĐÃ có sẵn dump — thêm sau run FAIL). |
| **D-2** | **P1** | 103 firefox.exe sống sót SAU khi go test thoát (headed×100); profile dir bị xóa sạch trong khi process sống. Tự chết dần 103→91 rồi đứng; phải force-kill 3 đợt. | `Close()` **không gửi BiDi `browser.close`** — chỉ đóng WS rồi `b.proc.Kill()` = TerminateProcess **PID cha duy nhất** ([browser.go:185-190](../../browser.go)); con/main không bị reap khi máy nghẽn (nghi launcher-process indirection của Firefox Windows — khớp láng giềng tĩnh **Z2-7** `os.ErrProcessDone`). |
| D-3 | P3 | 8/1414 navigate timeout 20s trong ~20s đầu (launch stampede headed) | Oversubscription thuần — stagger 250ms chưa đủ cho headed×100. |
| D-4 | P3 (đã fix trong harness) | Sampler đói: mẫu cách nhau 8 phút do 100× `tasklist`/mẫu | Đã sửa: 1 lần `tasklist` IMAGENAME/mẫu — run headless sau đó tick đều 15s. |

## Finding tĩnh — gắn nhãn bằng chứng động

| ID (sev tĩnh) | Nhãn | Ghi chú |
|---|---|---|
| Z1-4 (P3→**nâng P1**) | **CONFIRMED** (qua D-1, khớp bậc độ lớn; stack dump để chốt 100%) | Ứng viên chính của D-1 |
| Z2-7 (P3→P2) | **CONFIRMED-lân-cận** (qua D-2) | Kill parent-only + lỗi giả |
| Z3-1 (P1) | NOT-EXERCISED — **giữ P1 tĩnh** | Workload không dùng SetExtraHeaders; evidence code-level rõ |
| Z1-1, Z1-3, Z2-1, Z2-3 (P2) | NOT-OBSERVED (đường code CÓ chạy: write ×4.7k iters, tab churn ×100 browser, close ×200 browser — không triệu chứng ở headless) | Giữ P2, độ khẩn giảm |
| Z1-2, Z2-2, Z2-4, Z3-2, Z3-3, Z4-1..6 (P2) | NOT-EXERCISED | Workload không chạm (headers/WaitForNewPage/Drag/native/remote-path) |
| 18 P3 còn lại | NOT-EXERCISED / NOT-OBSERVED | Xem report vùng |

## Đề xuất fix (xếp hạng — KHÔNG implement trong plan này)

1. **D-2 (P1):** `Browser.Close` launch-path: (a) gửi BiDi `browser.close` graceful trước; (b) reap theo **process tree** (Windows Job Object khi spawn, hoặc `taskkill /T /F` fallback) thay vì `proc.Kill()` cha đơn lẻ; (c) verify-dead sau kill, trả lỗi thật thay vì nuốt. Files: `browser.go`, `browser_launch.go`.
2. **D-1/Z1-4 (P1):** `events.go:143-151` — không bỏ rơi cặp `wg.Wait` khi timeout: goroutine đợi phải thoát theo `b.closed`/ctx thay vì park vĩnh viễn; cân nhắc nâng drain budget theo tải. Verify bằng re-run headed×100 (harness đã có stack dump + orphan assert).
3. **Z3-1 (P1 tĩnh):** commit `hi.headers` bên trong `installMu`, rẽ nhánh theo `len(hi.headers)` — fix 5 dòng + test race install/remove.
4. **Z1-1 (P2):** expose `writeTimeout` knob qua Options (screenshot payload MB-scale × fleet là real risk).
5. Z2-1/Z2-3 (P2): sửa thứ tự signal drainer vs baseCancel + cho phép bỏ cleanup RPC khi saturated (trả lời câu hỏi mở Z2 trước khi code).
6. P2 còn lại (headers teardown, WaitForNewPage, input Drag/native/rand): gom vào cùng plan fix, mỗi cái kèm regression test.

**Khuyến nghị:** mở plan fix riêng (ước 4-6 phase, TDD) cho mục 1-4 trước; 5-6 sau khi user trả lời câu hỏi mở.

## Câu hỏi mở cho user (từ agents)

1. Z2-1: khi drainer bão hòa, chấp nhận BỎ cleanup RPC (page đã detach) không?
2. Z4-1: production có bao giờ chạy >1 Browser với BackendNative đồng thời không?
3. D-1 attribution 100%: chấp nhận 1 re-run headed×100 (~40 phút chiếm máy, sau khi fix) để verify?

## Giới hạn bằng chứng

Workload 120-180s/run, không phủ: SetExtraHeaders, WaitForNewPage, Drag/Tap, native backend, remote mouse path, soak nhiều giờ. Race detector chỉ bắt race THỰC SỰ xảy ra trong interleaving đã chạy. NOT-OBSERVED ≠ sạch tuyệt đối.
