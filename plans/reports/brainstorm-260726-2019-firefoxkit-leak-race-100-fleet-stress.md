# Brainstorm — Audit leak/race toàn bộ + stress test 100 tiến trình Firefox headed

**Ngày:** 2026-07-26 | **Phương án chốt:** C (song song hai mũi) | **User đã duyệt.**

## Bài toán

- Rà **toàn bộ** codebase firefoxkit tìm memory leak + race condition (audit mới, không tin mù 2 vòng audit cũ).
- Viết stress test mới workload hỗn hợp thực tế, chạy **trên máy này** với **100 tiến trình Firefox riêng biệt, headed**.
- Binary: `C:\Users\admin\.multiprofile\resources\browser\gpm_firefox_browser\FirefoxCore_v149\firefox.exe` (FF149, BiDi OK).
- Máy: Windows 10, 128GB RAM.

## Quyết định đã chốt (qua AskUserQuestion)

| Câu hỏi | Chốt |
|---|---|
| Hình dạng tải | 100 tiến trình Firefox riêng biệt (khớp production, tái dùng fleet harness) |
| Phạm vi audit | Audit mới toàn bộ, kể cả vùng chưa từng audit (`input/`, spawn, listeners) |
| Workload | Hỗn hợp thực tế: navigate + eval + click/type + tab churn + screenshot, sampling RSS/goroutine |
| Chế độ chạy | **Headed** (user chọn, đã cảnh báo desktop không dùng được + RAM 40-80GB) |
| Phương án tổ chức | C — audit tĩnh song song với viết harness, 1 lần chạy đắt, đối chiếu tĩnh↔động |

## Phương án đã đánh giá

- **A — Audit trước, stress sau:** chứng nhận sạch cuối cùng; nhưng chậm nhất, audit tĩnh lần 3 dễ ra ít finding mới. ❌
- **B — Stress-first:** bằng chứng thật dẫn đường; nhưng mỗi vòng lặp 100-process đắt, vùng workload không chạm sẽ không lộ. ❌
- **C — Song song hai mũi:** audit tĩnh (agents theo vùng) chạy trong lúc viết harness → chạy N=100 headed 1 lần với `-race` + sampling → đối chiếu finding tĩnh với số liệu động, xếp hạng theo bằng chứng. ✅ **CHỌN** — độ phủ cao nhất, đúng 1 run đắt.

## Thiết kế chi tiết

### Mũi 1 — Audit tĩnh (agents song song theo vùng)

| Vùng | File chính | Ghi chú |
|---|---|---|
| Z1 bidi core | `internal/bidi/client.go`, `transport.go`, `events.go`, `session.go`, `script.go`, `network.go` | Đã audit 2 lần — rà delta sau F1/F2/L1, verify fix đúng |
| Z2 browser lifecycle | `browser.go`, `browser_context_lifecycle.go`, `browser_spawn.go`, `browser_waitpage.go`, `browser_connect.go`, `browser_launch.go` | `WaitForNewPage` mới (d2ec92b) chưa audit |
| Z3 page layer | `page_listeners.go`, `page_console.go`, `page_dialog.go`, `page_downloads_api.go`, `page_headers.go`, `page_eval.go` | Handler đăng ký/hủy, sub/unsub |
| Z4 input (CHƯA TỪNG audit) | `input/cursor_engine.go`, `cursor_remote.go`, `cursor_tuning.go`, `keyboard_human.go`, `backend_bidi*.go`, `backend_native_windows*.go` | Ưu tiên cao — code mới nhất |

Checklist mỗi agent: goroutine có đường thoát; send vào channel đóng/đầy; map dưới lock; ctx propagation; timer/ticker leak; handler deregistration; pending-call cleanup khi Close; double-close; WaitGroup. Output: report mỗi vùng ≤80 dòng vào `plans/reports/`.

### Mũi 2 — Stress harness mới

- File mới: `integration_stress_fleet_workload_test.go` (build tag `stress`), tái dùng `launchFleet`/`fleetSize`.
- Knob mới: `FIREFOXKIT_HEADED=1` (default headless để không phá test cũ), `FIREFOXKIT_WORKLOAD_SECONDS` (default ~180).
- Workload mỗi browser (1 goroutine/browser, lặp tới hết duration): Navigate (data: URL xoay vòng) → Evaluate → click/type qua BiDi backend → mở/đóng tab → screenshot xác suất thấp (~1/10 vòng).
- Sampling mỗi 15s: `runtime.NumGoroutine`, `runtime.ReadMemStats` (Go heap), tổng RSS các PID firefox → log dạng bảng.
- Assertion cứng: zero error workload; goroutine delta sau khi Close hết fleet ≤ ngưỡng nhỏ; Go-side RSS growth có bound. Firefox RSS: log discovery, KHÔNG hard-assert (run đầu).
- Chạy với `-race`. Stagger launch ~250ms/instance (headed nặng hơn 50ms hiện tại).

### Chạy trên máy này

```
FIREFOXKIT_FIREFOX_PATH=C:\Users\admin\.multiprofile\resources\browser\gpm_firefox_browser\FirefoxCore_v149\firefox.exe
FIREFOXKIT_FLEET_N=100 FIREFOXKIT_HEADED=1
go test -tags stress -race -run Fleet_Workload -v -timeout 40m
```

Launch phase ước ~25-60s (stagger). Theo dõi RAM trong lúc chạy.

## Ràng buộc & sự thật khó nghe

- **Native Windows input backend KHÔNG stress song song được** — chỉ điều khiển cửa sổ focus. Workload dùng BiDi backend. Native input = test riêng 1 cửa sổ (ngoài scope run này).
- 100 headed: desktop gần như không dùng được suốt run; RAM ước 40-80GB — OK trên 128GB nhưng phải theo dõi; GDI/window limit Windows có thể là biến số.
- `-race` làm Go-side chậm ~2-10x — chấp nhận, đây là mục đích.
- Profile temp dir × 100 — verify cleanup sau Close (đã có test `TestStress_ProfileDir_NoLeak` cho case nhỏ).

## Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| RAM cạn giữa run | Stagger launch; sampling RSS sớm; abort loud nếu launch fail |
| Headed 100 cửa sổ vẽ chồng nhau gây noise CPU | Chấp nhận — user chọn headed có chủ đích; kết quả vẫn conservative |
| Audit tĩnh ra finding lớn SAU run | Có thể cần run xác nhận thứ 2 (đã báo user khi chọn C) |
| Fix cũ F1/F2/L1 đã vào code nhưng plan.md cũ ghi pending | Sửa metadata plan cũ khi tiện — không tin status file, tin code |

## Tiêu chí thành công

1. Race detector sạch qua toàn bộ run 100×headed.
2. Goroutine về mức nền sau khi đóng hết fleet (delta ≤ ngưỡng).
3. Đường RSS (Go + Firefox tổng) phẳng hoặc tăng có bound trong duration.
4. Báo cáo hợp nhất: finding tĩnh × bằng chứng động, xếp hạng ưu tiên fix.

## Bước tiếp theo

`/plan` từ báo cáo này → phases: (1) audit agents song song 4 vùng, (2) viết harness workload, (3) chạy run 100 headed + thu số liệu, (4) báo cáo hợp nhất + đề xuất fix (fix thật nằm plan sau nếu có finding).

## Câu hỏi chưa chốt

- Không.
