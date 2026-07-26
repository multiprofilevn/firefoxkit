# Phase 04 — Write timeout knob (Z1-1)

**Status:** completed · TDD (red → green) · 2026-07-27

## Thay đổi

| File | Nội dung |
|---|---|
| `options.go` | field `writeTimeout` + `WithWriteTimeout(d)` (0 → default, âm → bỏ qua) |
| `browser_launch.go` / `browser_connect.go` | `WriteTimeout: cfg.writeTimeout` vào `bidi.Options` (đúng pattern `DefaultCallTimeout`) |
| `internal/bidi/client.go` | `Options.WriteTimeout` → `dial(..., opts.WriteTimeout)` |
| `internal/bidi/transport.go` | `dial` nhận `writeTimeout`; tách `effectiveWriteTimeout()` (0/âm → `defaultWriteTimeout` 10s) |
| `README.md` | 1 dòng bảng options |

Test mới:
- `internal/bidi/transport_write_timeout_test.go` — short timeout (50ms) cắt đúng hạn; long timeout (3s) không cắt sớm + conn còn sống; zero/âm → default; `Dial(Options{WriteTimeout})` chạm tới transport qua ws server thật (httptest), unset → default.
- `options_write_timeout_test.go` — default 0, set 30s, 0 khôi phục default, âm bị bỏ qua.

## Quyết định

- **Không đặt default trong `defaultConfig()`** (giữ 0): một nguồn sự thật duy nhất cho 10s là `defaultWriteTimeout` trong bidi. Khác `commandTimeout` vì ở đó 0 = tắt backstop, còn ở đây 0 = dùng default.
- **`dial()` nhận `writeTimeout` qua tham số** thay vì gán sau khi dựng — tránh ghi field sau khi `readLoop` khởi động (race-clean).
- **Tách `effectiveWriteTimeout()`** khỏi `writeJSON` để test case "zero → default" không phải chờ 10s.
- Không đổi semantics F2: write vẫn chạy dưới internal ctx, chỉ tham số hoá thời lượng.

## Gates

| Gate | Kết quả |
|---|---|
| `go vet ./...` | pass |
| `go build ./...` | pass |
| `go test -race ./... -count=1` | pass (root 20.4s, input 4.0s, bidi 18.1s) |
| `go vet -tags stress ./...` / `go build -tags stress ./...` | pass |
| `go vet -tags wsintegration ./...` | pass |
| `go doc ... WithWriteTimeout` | render đúng, nêu mutex-wait + teardown + gợi ý ~30s |

## Vấn đề còn lại

Không. Phase 5 (native gesture lock) không đụng file nào ở trên.
