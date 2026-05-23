# Go library review checklist

Production-readiness checklist distilled from firefoxkit's first review pass. Apply to any Go library that wraps a long-lived external resource (browser, DB, RPC server, OS).

## Resource leaks

- [ ] **`syscall.NewCallback` is hoisted to package-level `sync.Once`** — never inside a per-call function. The Go callback table is fixed-size (~1024 on amd64) and entries are never GC'd. See `docs/native-input-calibration.md` for the pattern.
- [ ] **Event subscriptions** registered per-object (per-Page, per-Connection) **must be unsubscribed in the object's Close/Cleanup path**. Closures capturing the parent keep the parent alive — silent memory leak.
- [ ] **Server-side state** allocated via RPC (intercepts, sessions, watchers) **must be released on local-side close**. Don't rely on the server's GC.
- [ ] **Background goroutines reading from external pipes** (stderr, stdout, sockets) **must exit when the resource is closed**, not just when the caller stops reading.

## Context propagation

- [ ] **Every public RPC method takes `ctx context.Context` as first arg.** No `context.Background()` synthesized internally for caller-initiated work.
- [ ] **Polling loops derive per-iteration sub-contexts from the outer ctx**, not from `context.Background`. Check both `ctx.Err()` AND `errors.Is(err, context.DeadlineExceeded)` — they're distinct paths.
- [ ] **Hardcoded internal timeouts** (`time.Second * 5` inside a method) are a smell. The caller's ctx is the source of truth. Push timeouts to the caller via `WithTimeout`.

## Event handlers + dispatcher

- [ ] **Event handlers don't perform synchronous RPCs by default** unless the bus is multi-goroutine. A single dispatcher goroutine + a handler calling `client.Send(...)` means every other event is blocked on that one RPC.
- [ ] **Handler panics are recovered** — one bad listener should not kill the dispatcher.
- [ ] **Backpressure policy** documented: bounded channel + drop-oldest is fine if Wait() callers can survive missed events. If they can't (must-deliver semantics), use unbounded + monitor.

## Concurrency

- [ ] **No write-lock held across an RPC.** Acquire lock → mutate local state → release → call out. Long-held locks block every concurrent reader.
- [ ] **Stress tests under `-race`** for any goroutine that touches shared state. CGO_ENABLED=1 on the race-detector runner.
- [ ] **Late-arrival paths** tested: response arriving after ctx.Cancel, Close racing in-flight Send, drainPending while reader is delivering.
- [ ] **Object lifecycle flag** (`closed atomic.Bool` on Page/Conn/Session) checked at the top of every public method. Returns ErrClosed fast.

## Security

- [ ] **Credentials are never echoed in error messages.** `net.SplitHostPort("user:pass@host:port")` puts the full input in the error string. Parse, redact, then wrap.
- [ ] **Credentials are never written to disk** unless explicitly opt-in. Profile files / prefs.js / config caches written to user-supplied dirs persist creds.
- [ ] **Logged event payloads** are scrubbed of headers (Authorization, Cookie), URLs (may contain tokens in query strings), and cookie values.

## Error mapping

- [ ] **Sentinel errors** (`ErrInvalidConfig`, `ErrTimeout`, ...) **mean what their name says**. Don't `%w: ErrInvalidConfig` for a runtime RPC failure — it pollutes `errors.Is` consumers.
- [ ] **`%w` wrapping is consistent** — every fmt.Errorf that should be type-introspectable uses `%w`, not `%v`.
- [ ] **`errors.Is(err, sentinel)` works at every public return point.** Spot-check 5-10 sites per release.

## API surface contract

- [ ] **Same signature pattern across the surface.** If Evaluate takes ctx, Navigate takes ctx. If Cookies returns `(value, err)`, Storage returns `(value, err)`. Inconsistency is a paper cut per call site.
- [ ] **Breaking changes documented** explicitly. Migration guides exist (see `docs/migration-from-chromekit.md` for shape).
- [ ] **Public types versus internal types** clearly separated. `internal/...` package for everything callers shouldn't import.

## Tests

- [ ] **Pure functions have unit tests.** Action builders, encoders, parsers — no excuse for skipping.
- [ ] **Integration tests** gated by env var (`FIREFOXKIT_FIREFOX_PATH`) + build tag (`-tags=integration`). CI runs them on installed runtime.
- [ ] **Lock-down tests** for size-sensitive structs: `unsafe.Sizeof(WinAPIStruct{}) == 40`. Catch refactor-induced padding changes.
- [ ] **Cross-compile sanity** (`GOOS=darwin go build ./...`, `GOOS=linux go build ./...`) every commit.
- [ ] **Race detector** runs on at least one CI matrix entry. CGO_ENABLED=1.

## Documentation

- [ ] **README has Install + Quickstart + Core API table + Options + Limitations**. The Limitations section is the most-read.
- [ ] **Migration guide** if porting from a sibling library.
- [ ] **Architecture diagram** even if simple ASCII — first thing a contributor reads.
- [ ] **Gotchas doc** with the surprising things (callback exhaustion, ctx rules, etc.).

## Origin

Distilled 2026-05-23 from review of firefoxkit (`plans/reports/review-260523-1420-firefoxkit-production.md`).
