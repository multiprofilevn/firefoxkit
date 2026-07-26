---
phase: 4
title: "Write timeout knob"
status: completed
priority: P2
effort: "1h"
dependencies: []
---

# Phase 4: Write timeout knob

## Overview

Fix Z1-1 (audit Z1): `defaultWriteTimeout` 10s cố định, không cấu hình được (`dial` để
`writeTimeout: 0` → luôn default). coder/websocket tính cả thời gian chờ write-mutex vào ctx đó và
`close()` cả conn khi hết hạn giữa frame → payload lớn (screenshot MB-scale × fleet) có thể sập cả
session. Expose knob theo pattern options hiện có.

## Requirements

- Functional: option mới `WithWriteTimeout(d time.Duration)` trong `options.go` (pattern y hệt `WithCommandTimeout` nếu có / các With* hiện hữu) → plumb qua `Launch`/`Connect` → `bidi.Dial` Options → `transport.writeTimeout`. `0`/không set → giữ default 10s (hành vi không đổi).
- Non-functional: doc comment nêu rõ trade-off (frame write bao gồm chờ mutex; fleet lớn + screenshot → cân nhắc 30s).

## Related Code Files

- Modify: `options.go`, `browser_launch.go`/`browser_connect.go` (plumb), `internal/bidi/client.go` (Options), `internal/bidi/transport.go` (nhận giá trị)
- Modify (test): `internal/bidi/transport_write_test.go` — case custom timeout

## Existing code audit

Audit Z1 (`../260726-2019-firefoxkit-leak-race-audit-100-fleet-stress/reports/audit-z1-bidi-core.md`, finding Z1-1) + pattern options đã có sẵn (`options.go` — `WithNativeWindow`, `WithCursorTuning`…) — thay vai trò reuse-scout. Verdict: **REUSE-EXTEND** options pipeline hiện có, không cơ chế mới.

## Reuse strategy

Copy đúng pattern plumb của `DefaultCallTimeout` (F1 đã plumb tương tự từ options → client) — một đường ống, hai knob.

## Implementation Steps (TDD)

1. **Test trước**: `transport_write_test.go` — set writeTimeout ngắn (50ms) + fake conn chậm → write trả lỗi đúng hạn; set dài → không cắt sớm; không set → default 10s giữ nguyên.
2. Implement option + plumb; `go doc` render đúng.
3. `go test -race ./...` + vet/build stress tag.

## Success Criteria

- [x] Option mới hoạt động end-to-end (Launch → transport), default không đổi hành vi
- [x] Test 3 case xanh
- [x] `go test -race ./...` xanh

## Risk Assessment

- Không đụng semantics F2 (write vẫn dưới internal ctx) — chỉ tham số hóa thời lượng.
