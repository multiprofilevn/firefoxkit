# Audit Z1 — bidi core (leak/race, static)

Scope: `internal/bidi/{client,transport,events,session,script,network,message}.go` (read-only).
Cross-referenced callers: `browser_connect.go`, `browser_launch.go`, `options.go`, `page.go`,
`page_headers.go`, `page_listeners.go`, `page_downloads_api.go`, `browser_context_lifecycle.go`,
plus `coder/websocket@v1.8.14` `close.go` / `write.go`.

## Fix re-verification

- **F1 (Send backstop) — VERIFIED GOOD.** `client.go:110-116` applies the deadline only when the
  caller has none; `client.go:131 defer c.cancelPending(id)` removes the map entry on *every* exit
  (timeout, write error, ErrClientClosed). A late response then hits `client.go:259 ok==false` →
  logged orphan, dropped. The channel is `make(chan *frame, 1)` (`client.go:119`) and is deleted
  from `pending` under `c.mu` *before* `ch <- f` (`client.go:253-260`), so no send-on-closed and no
  send-on-dead-channel is reachable. Wired in production at `browser_connect.go:36` /
  `browser_launch.go:99` with 60s default (`options.go:65`); 0 = documented opt-out.
- **F2 (write under internal ctx) — VERIFIED GOOD, with residual Z1-1.** `transport.go:61` drops the
  caller ctx (`_ context.Context`) entirely; the only other conn users are `readJSON` (driven with
  `context.Background()` at `client.go:224`) and `dial`. No caller-ctx path can reach `conn.Write`.
- **Pending cleanup on Close — CLEAN.** `shutdown()` closes `c.closed` *before* `finish()` runs
  (`client.go:180-186`, `client.go:217-221`), and `Send`'s registration checks `c.closed` while
  holding `c.mu` (`client.go:121-129`), so no entry can be added after `drainPending` swaps the map.
  Every parked `Send` unblocks via `close(ch)`, `<-c.closed`, or ctx. No park-forever path found.
- **Handler maps — CLEAN.** `dispatch` sends under `RLock`, `remove`/`closeAll` delete under `Lock`
  before `close(entry.ch)` (`events.go:102-152`) — no close/send race. Every production
  `Subscribe` has a deregistration: `page.go:118-153` (downloads + tracked subs),
  `page_headers.go:143-146`; `browser_context_lifecycle.go:38` is client-lifetime by design.

## Findings

| ID | file:line | type | sev | evidence | fix | conf |
|---|---|---|---|---|---|---|
| Z1-1 | transport.go:66-72, 93 | deadlock | P2 | The internal write ctx is a fixed `defaultWriteTimeout = 10s` and `coder/websocket` counts write-mutex wait inside it (`write.go:147,191 mw.writeMu.lock(mw.ctx)`) then **closes the whole conn** on mid-frame expiry (`write.go:322 _ = c.close()`). All concurrent `Send`s on one client serialize on that one mutex, so a burst of large frames (64MB read limit implies MB-scale writes) can blow 10s and tear down the session for every page. `dial` hardcodes it — `transport.go:48 return &wsTransport{conn: conn}` leaves `writeTimeout: 0`; only `NewClientForTest` can set it. | Expose `Options.WriteTimeout` (plumb into `dial`) and/or scale the budget with `len(data)`. | med |
| Z1-2 | events.go:157-168 + page_headers.go:84-124 | memory-leak | P2 | `dispatch` drops on a full 256 queue with only `logger.Warnf` (`events.go:163-166`). For `network.beforeRequestSent` the dropped event is an **intercepted (paused)** request, so `ContinueRequest` (`page_headers.go:117`) never fires and the request hangs browser-side until Firefox times out. The handler issues a synchronous 5s RPC per event on a single worker (`page_headers.go:115-116`), so queue fill is reachable under load. | Never drop for intercept-bearing methods: block-with-deadline, or on drop spawn a bounded fallback `ContinueRequest`. | high |
| Z1-3 | events.go:158-159 | unbounded-growth | P2 | `b.handlers[method]` is keyed on method **only**; per-context filtering happens after queueing (`page_headers.go:89`, `page_listeners.go:50 if ev.Context != p.contextID`). With N pages, one busy context's events consume the 256-slot quota of all N subscriptions, causing drops (→ Z1-2) on unrelated pages. Fan-out cost is O(N) per event. | Accept an optional context filter at `subscribe()` and skip non-matching entries in `dispatch` before enqueue. | high |
| Z1-4 | events.go:143-151 | goroutine-leak | P3 | `go func() { b.wg.Wait(); close(done) }()` is abandoned when the `workerDrainTimeout` branch (`events.go:147`) is taken — it parks on `wg.Wait()` forever together with the stuck worker. Reachable whenever a handler blocks (user `DialogHandler` at `page_listeners.go:56-61` runs on the worker) or `Close()` is called from a handler. 100-client fleet → up to 100 permanently parked pairs. | Document as accepted, or give workers a cancel channel so `wg` can actually reach zero. | high |
| Z1-5 | events.go:121-125 | deadlock | P3 | The already-closed branch does a bare `b.wg.Wait()` with **no** timeout, unlike the guarded wait 25 lines below. Currently unreachable only because `finishOnce` (`client.go:203`) serializes the single caller — any future second caller parks forever. | Route both branches through the same bounded wait helper. | high |
| Z1-6 | client.go:134-136 | race | P3 | When `Close()` races an in-flight write, `Send` returns `fmt.Errorf("bidi: write %s: %w", method, err)` wrapping the transport error — **not** `ErrClientClosed`. Callers doing `errors.Is(err, ErrClientClosed)` to detect shutdown mis-classify it as a transient failure and may retry against a dead client. | After a write error, `select { case <-c.closed: return ErrClientClosed; default: }` before wrapping. | high |
| Z1-7 | client.go:194 + transport.go:83-85 | deadlock | P3 | `shutdown` calls `conn.Close()`, i.e. the full close handshake, bounded by two independent 5s timeouts (`close.go:185`, `close.go:199`). On the reader-fatal path (`client.go:219`, peer already dead) this burns up to ~10s *before* `close(c.readerDone)`, and `Client.Close()` blocks on `<-c.readerDone` (`client.go:183`) the whole time. Serial fleet teardown multiplies this. | Use `CloseNow()` on the reader-exit path (`client.go:219`); keep the handshake only for user-initiated `Close`. | med |
| Z1-8 | client.go:281-286 | (doc defect) | P3 | Comment says "Deliver a synthetic error frame so blocked Sends return ErrClientClosed" but the code only `close(ch)`; `_ = id` is a leftover no-op and the loop does not need the key. Behaviour is correct (`client.go:140-142` handles `nil`), the comment is not. | `for _, ch := range pending { close(ch) }` and fix the comment. | high |

## Clean areas (checked, nothing found)

- `session.go`, `script.go`, `network.go`, `message.go`: pure marshalling + thin `Send` wrappers —
  no goroutines, timers, channels, or shared mutable state. `DecodeRemoteValue` recursion
  (`script.go:110-196`) is bounded by input depth only; not a leak on trusted browser input.
- `go` statements: exactly three (`client.go:92`, `events.go:93`, `events.go:144`). The first two
  have clean exit paths (conn close → reader returns; channel close → worker returns); the third is
  Z1-4.
- No `time.Timer`/`Ticker` in the package (only `time.After` at `events.go:147` — Z1-4).
- `sync.WaitGroup`: `Add` under `mu.Lock` guarded by `b.closed` (`events.go:79-91`), `Done` deferred
  in the worker (`events.go:94`) — balanced, no Add-after-Wait.
- Double-`Close` safe via `shutdownOnce`/`finishOnce` (`client.go:49-50`); `Unsubscribe` idempotent
  via `done.Swap(true)` (`events.go:40`).
- `c.defaultCallTimeout` written post-construction (`client.go:75`) after `go c.readLoop()` — not a
  race: `readLoop` never reads it, and user goroutines are created after `Dial` returns.

**Status:** DONE

F1 and F2 both hold as implemented — every `Send` path gets a deadline and its pending entry is
always reclaimed, and no caller ctx can reach `conn.Write`. The residual fleet-stress risks are the
fixed, unconfigurable 10s write budget that still tears down the shared socket under write-mutex
contention (Z1-1), and the method-only event keying whose 256-slot drop policy silently orphans
intercepted requests (Z1-2/Z1-3).
