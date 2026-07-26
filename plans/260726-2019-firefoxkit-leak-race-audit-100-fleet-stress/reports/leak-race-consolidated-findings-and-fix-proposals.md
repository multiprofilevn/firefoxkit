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
| **D-1** | **P1** | Đóng 100 browser headed → 264 goroutine không thoát (run 1) / 234 (re-run chẩn đoán). Không tái hiện N=10 headed / N=100 headless. | **ĐÃ CHỐT bằng stack dump (re-run):** 234 = **78 browser orphan × đúng 3 goroutine**: (a) exec stderr copier `os/exec.(*Cmd).writerDescriptor.func1` (exec.go:602), (b) scanner `discoverBiDiURL.func1` ([browser_launch.go:323](../../browser_launch.go)), (c) `Launch.func2` `cmd.Wait()` ([browser_launch.go:136](../../browser_launch.go)) — cả 3 park trên pipe/Wait của **process Firefox chưa chết**. → **D-1 là hệ quả trực tiếp của D-2, KHÔNG phải Z1-4** (không có frame `events.go` nào trong dump — goroutine bidi thoát sạch). Fix D-2 là hết D-1. |
| **D-2** | **P1** | Run 1: 103 firefox.exe sống sót sau go test; re-run: **78/100 browser để lại 125 process orphan** — dù `Close()` trả **nil cho cả 100 browser** (`proc.Kill()` "thành công" mà process không chết; 22 browser còn lại chết bình thường — phi tất định theo tải). Profile dir vẫn bị xóa sạch. | `Close()` **không gửi BiDi `browser.close`** — chỉ đóng WS rồi `b.proc.Kill()` = TerminateProcess **PID cha duy nhất** ([browser.go:185-190](../../browser.go)) và không verify-dead; dưới nghẽn headed×100 process sống sót trong khi Kill báo nil (nghi zombie-handle/launcher indirection — cần pin khi implement). Fix bắt buộc: BiDi `browser.close` graceful → **process-tree kill** (Job Object / `taskkill /T /F`) → **verify-dead** + trả lỗi thật. |
| D-3 | P3 | 8/1414 navigate timeout 20s trong ~20s đầu (launch stampede headed) | Oversubscription thuần — stagger 250ms chưa đủ cho headed×100. |
| D-4 | P3 (đã fix trong harness) | Sampler đói: mẫu cách nhau 8 phút do 100× `tasklist`/mẫu | Đã sửa: 1 lần `tasklist` IMAGENAME/mẫu — run headless sau đó tick đều 15s. |

## Finding tĩnh — gắn nhãn bằng chứng động

| ID (sev tĩnh) | Nhãn | Ghi chú |
|---|---|---|
| Z1-4 (P3, từng nghi P1) | **NOT-OBSERVED — hạ về P3 lý thuyết** (stack dump re-run: 0 frame events.go; goroutine bidi thoát sạch cả ở headed×100) | Giả thuyết ban đầu SAI — bằng chứng động sửa lại |
| Z2-7/Z2-8/Z2-9 (P3→**P1 cụm D-2**) | **CONFIRMED** (dump: cmd.Wait + stderr scanner + exec copier park đúng như Z2-8/Z2-9 mô tả khi process không chết) | Cụm launch/reap chính là thủ phạm |
| Z3-1 (P1) | NOT-EXERCISED — **giữ P1 tĩnh** | Workload không dùng SetExtraHeaders; evidence code-level rõ |
| Z1-1, Z1-3, Z2-1, Z2-3 (P2) | NOT-OBSERVED (đường code CÓ chạy: write ×4.7k iters, tab churn ×100 browser, close ×200 browser — không triệu chứng ở headless) | Giữ P2, độ khẩn giảm |
| Z1-2, Z2-2, Z2-4, Z3-2, Z3-3, Z4-1..6 (P2) | NOT-EXERCISED | Workload không chạm (headers/WaitForNewPage/Drag/native/remote-path) |
| 18 P3 còn lại | NOT-EXERCISED / NOT-OBSERVED | Xem report vùng |

## Đề xuất fix (xếp hạng — KHÔNG implement trong plan này)

1. **D-2 → hết luôn D-1 (P1 duy nhất):** `Browser.Close` launch-path: (a) gửi BiDi `browser.close` graceful trước; (b) reap theo **process tree** (Windows Job Object khi spawn, hoặc `taskkill /T /F` fallback) thay vì `proc.Kill()` cha đơn lẻ; (c) **verify-dead** sau kill (poll exit tối đa vài giây), trả lỗi thật thay vì nil giả. Khi process chết thật, 3 goroutine/browser (exec copier + `discoverBiDiURL` scanner + `Launch.func2` Wait) tự thoát → D-1 biến mất. Files: `browser.go`, `browser_launch.go`. Verify: re-run headed×100 (harness có sẵn stack dump + orphan assert).
2. ~~D-1/Z1-4~~ **HỦY** — stack dump chứng minh Z1-4 không phải thủ phạm (0 frame events.go); giữ Z1-4 ở backlog P3 lý thuyết.
3. **Z3-1 (P1 tĩnh):** commit `hi.headers` bên trong `installMu`, rẽ nhánh theo `len(hi.headers)` — fix 5 dòng + test race install/remove.
4. **Z1-1 (P2):** expose `writeTimeout` knob qua Options (screenshot payload MB-scale × fleet là real risk).
5. Z2-1/Z2-3 (P2): sửa thứ tự signal drainer vs baseCancel + cho phép bỏ cleanup RPC khi saturated (trả lời câu hỏi mở Z2 trước khi code).
6. P2 còn lại (headers teardown, WaitForNewPage, input Drag/native/rand): gom vào cùng plan fix, mỗi cái kèm regression test.

**Khuyến nghị:** mở plan fix riêng (ước 4-6 phase, TDD) cho mục 1-4 trước; 5-6 sau khi user trả lời câu hỏi mở.

## Câu hỏi mở — ĐÃ CHỐT (user trả lời 2026-07-26)

1. Z2-1: **USER DUYỆT — BỎ cleanup RPC khi drainer bão hòa** (drop + đếm counter). `contextDestroyed` = context đã chết phía Firefox, server-side state chết theo, RPC chỉ là belt-and-suspenders.
2. Z4-1: **CÓ — production chạy native backend tối đa 9 browser đồng thời** (chuột thật, >9 không đủ chuột) → Z4-1 là rủi ro THẬT, plan fix phải thêm fleet-wide gesture lock. User yêu cầu kèm sắp xếp cửa sổ + scale (như chrome window-position/size/scale-factor): firefoxkit ĐÃ CÓ `WithNativeWindow(x,y,scale)` + `SetWindowBounds` + `WindowScale`; plan fix bổ sung tiện ích scale-factor pref (`layout.css.devPixelsPerPx`) + verify tile 9 cửa sổ native. (Đã lưu memory fact.)
3. Re-run verify headed×100 sau fix: user ủy quyền — **Claude quyết: CÓ**, đưa thành phase verify cuối của plan fix (~40 phút chiếm máy, chạy khi user rảnh).

## Giới hạn bằng chứng

Workload 120-180s/run, không phủ: SetExtraHeaders, WaitForNewPage, Drag/Tap, native backend, remote mouse path, soak nhiều giờ. Race detector chỉ bắt race THỰC SỰ xảy ra trong interleaving đã chạy. NOT-OBSERVED ≠ sạch tuyệt đối.
