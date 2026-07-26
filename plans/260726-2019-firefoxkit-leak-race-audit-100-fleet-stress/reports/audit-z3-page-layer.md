# Audit Z3 — Page Layer (leak/race, static)

Scope: `page_listeners.go`, `page_console.go`, `page_dialog.go`, `page_downloads_api.go`,
`page_headers.go`, `page_eval.go`, `page_action.go`, `page_wait.go`, `page_navigate.go`.
Context read: `page.go`, `browser.go`, `browser_context_lifecycle.go`, `browser_pages.go`,
`internal/bidi/events.go`, `internal/bidi/client.go`, `concurrency_hardening_test.go`.
Read-only — no `.go` file modified.

## Findings

| ID | file:line | Type | Sev | Evidence | Fix (1 line) | Conf |
|---|---|---|---|---|---|---|
| Z3-1 | page_headers.go:44-65 | Race → permanent interceptor | P1 | Header map is committed under `hi.mu` (44-49) but the install/remove branch (56-65) runs later under `installMu` and keys off the **argument** `headers`, not the committed `hi.headers`. Interleave `SetExtraHeaders(A)` / `SetExtraHeaders(nil)`: nil-caller wins `installMu` first → `removeHeaderInterceptor` no-ops; A-caller then installs. Final state = interceptor installed + `hi.headers` empty → user's removal silently lost and every request pays intercept + `ContinueRequest` round-trip for the page's life. `TestSetExtraHeaders_ConcurrentInstall_SingleIntercept:46` only uses non-empty maps, so this is uncovered. | Move the `hi.headers` write inside `installMu` and branch on `len(hi.headers)`, not on the argument. | High |
| Z3-2 | page_headers.go:83,133-150 | Asymmetric teardown / unbounded slice | P2 | Install calls `p.trackServerEvent("network.beforeRequestSent")` (83) on **every** install; `removeHeaderInterceptor` (133-150) drops the intercept + local sub but never calls `UnsubscribeEvents` and never removes the tracked name. `trackServerEvent` (page.go:87-91) appends unconditionally → N install/remove cycles leave N duplicate entries and Firefox keeps streaming `network.beforeRequestSent` for that context with zero subscribers until page cleanup. 100-page fleet ⇒ per-request WS traffic with no consumer. | In `removeHeaderInterceptor`, `UnsubscribeEvents` the event and drop it from `p.serverEvents`; make `trackServerEvent` dedupe. | High |
| Z3-3 | page_wait.go:40-47 | Doomed-RPC storm after ctx destroyed | P2 | `pollUntil` swallows every non-`context.Canceled` error from `check` and keeps polling. `evaluateCtx` against a destroyed browsing context returns a wrapped `evalError` (element_handle.go:78-91), never `Canceled` — so `WaitForSelector(sel, 60s)` on a tab Firefox already destroyed issues an `Evaluate` RPC every ≤500ms (each with a 5s attempt ctx, page_wait.go:37) for the full 60s. Under fleet churn many waiters do this at once. | Break out of the loop on `ErrPageNotFound` / "no such frame" instead of treating it as transient. | Med-High |
| Z3-4 | page_downloads_api.go:63-74 | Waiter parked past page death | P3 | `cleanup` (page.go:123-131) nils `dl.sub` but never closes or signals `dl.pending`; `Wait` selects only on `pending` and `opCtx`, which derives from the **browser** ctx (page.go:181-187). `ClosePage` therefore leaves an in-flight `Wait` parked for the full timeout (default 30s, line 58-60) holding a `*Page` ref, then returns `ErrTimeout` instead of a page-closed error. | Add a per-page `closed` channel to the `Wait` select and return `ErrPageNotFound`. | High |
| Z3-5 | page_headers.go:117 | Uncancellable RPC on dispatcher worker | P3 | The intercept handler issues `ContinueRequest` on `context.Background()` + 5s, not `p.ctx()`. `Browser.Close`'s `baseCancel` (browser.go:140) cannot unblock it, so the `network.beforeRequestSent` worker stays parked through the graceful phase; it is bounded only by `Send`'s `<-c.closed` (internal/bidi/client.go:164), which fires after `client.Close()` — i.e. after `gracefulCloseBudget`. Adds latency per browser close under load. | Derive the RPC ctx from `p.ctx()` with the 5s cap. | High |
| Z3-6 | page_listeners.go:20,83 | Missing deregistration path | P3 | `OnDialog(nil)` / `OnConsole(nil)` only `Store` a nil handler; `dialogSubscribed`/`consoleSubscribed` stay true, the local sub + its worker goroutine (internal/bidi/events.go:93) and the server-side `session.subscribe` for `log.entryAdded` / `browsingContext.userPromptOpened` all survive until `cleanup`. 100-page fleet with `OnConsole` then `OnConsole(nil)` ⇒ 100 live console streams + 100 idle worker goroutines the caller believes are off. | Add `OffConsole`/`OffDialog` (or make nil-handler tear the sub + server event down). | High |
| Z3-7 | page_downloads_api.go:109 | Fragile blocking send | P3 | Drop-oldest path does a non-blocking drain (105-108) then an **unconditional** send on `dl.pending` from the dispatcher worker. Safe today only because the bus gives this subscription a single worker and `Wait` is the only other consumer; a second producer (or a future second subscription on the same channel) parks the worker indefinitely. | Make the retry send `select`/`default` too. | Med |

## Clean areas

- `page.go:98-165` cleanup — `cleanedUp.Swap(true)` precedes every lock acquisition and each lazy-subscribe path re-checks it under the *same* lock (`listenerMu` / `installMu` / `dl.mu`); no orphan-subscription window. Idempotent across `contextDestroyed`, `ClosePage`, `Browser.Close`.
- `page_navigate.go` — no goroutines/timers; `defer cancel()` on every path (15,33,46,53); complete error mapping in `wrapNavErr:78`.
- `page_action.go:54-67` `Page.Wait` — `time.NewTimer` + `defer t.Stop()`, ctx honored.
- `page_eval.go` — pure ctx-propagating RPC pass-throughs; no state, no goroutines.
- `page_wait.go:57` `time.After` — go.mod declares `go 1.26.2`, so the unreferenced timer is GC'd immediately; not a leak.
- `internal/bidi/events.go:102-116` — `remove()` deletes the entry under `mu.Lock` *before* `close(ch)`, `dispatch` sends non-blockingly under `RLock` ⇒ no send-on-closed, no dispatcher stall. `wg.Add` under `mu` before goroutine start with deferred `Done` (90-98) ⇒ balanced.
- Callback re-entrancy — a user handler calling `p.Close()` (browser_pages.go:74) unsubscribes its own worker via `remove()`, which does not wait; `Browser.Close()` from a handler is bounded by `workerDrainTimeout` (internal/bidi/events.go:14,145-151). No self-deadlock found.
- `p.mu` discipline — `url`/`title` reads/writes consistently guarded (page.go:202-243, page_navigate.go:25-27, page_wait.go:111-126).
- `p.hdr` read from the intercept handler (page_headers.go:92) is race-free: written once under `initMu` before `Subscribe` creates the worker (happens-before via goroutine start), never re-written.

## Test-coverage gaps (Z3-specific)

Not covered by `concurrency_hardening_test.go`: (a) `SetExtraHeaders(map)` racing `SetExtraHeaders(nil)` → Z3-1; (b) repeated install/remove cycles asserting one `network.unsubscribe` per install → Z3-2; (c) `Downloads().Wait` racing `cleanup` → Z3-4; (d) `pollUntil` against a destroyed context → Z3-3.

## Unresolved questions

- Z3-3 severity depends on the exact BiDi error Firefox returns for a destroyed context; the string was not verified against a live peer (static audit only).

**Status:** DONE
Seven findings in the page layer — one P1 (`SetExtraHeaders` install/remove ordering can permanently pin an interceptor the user asked to remove), two P2 (header server-event teardown is asymmetric and unbounded; `pollUntil` polls dead contexts for the full timeout), four P3. Page cleanup's lock ordering, the bidi event bus, and the navigate/eval/action paths are clean.
