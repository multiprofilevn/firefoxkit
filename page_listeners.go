package firefoxkit

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// OnDialog registers a handler invoked for every alert/confirm/prompt raised
// by the page. Replaces any previous handler — only one handler slot is kept.
// Pass nil to revert to auto-dismiss (keeps the tab from hanging on alerts).
//
// The handler receives a DialogEvent and must return a DialogAction. The zero
// value of DialogAction (Accept=false) dismisses the dialog. Matches the
// chromekit OnDialog signature exactly.
//
// Subscription is created at most once per page; subsequent calls only replace
// the handler slot. Released automatically when the page is closed.
func (p *Page) OnDialog(handler func(DialogEvent) DialogAction) {
	p.dialogHandler.Store(&handler)

	p.dialogOnce.Do(func() {
		ctx := p.ctx() // capture before goroutine so Close() cancel is honoured
		if err := p.browser.client.SubscribeEvents(ctx, bidi.SubscribeParams{
			Events:   []string{"browsingContext.userPromptOpened"},
			Contexts: []string{p.contextID},
		}); err != nil {
			if p.log != nil {
				p.log.Warnf("firefoxkit: OnDialog subscribe: %v", err)
			}
			// Reset Once so a future call can retry after a transient failure.
			p.dialogOnce = sync.Once{}
			return
		}
		p.trackSub(p.browser.client.Subscribe("browsingContext.userPromptOpened", func(params json.RawMessage) {
			var ev bidi.UserPromptOpenedEvent
			if err := json.Unmarshal(params, &ev); err != nil {
				if p.log != nil {
					p.log.Warnf("firefoxkit: decode userPromptOpened: %v", err)
				}
				return
			}
			if ev.Context != p.contextID {
				return // event for another tab
			}

			action := DialogAction{Accept: false} // default: dismiss
			if hp := p.dialogHandler.Load(); hp != nil && *hp != nil {
				action = (*hp)(DialogEvent{
					Type:          DialogType(ev.Type),
					Message:       ev.Message,
					DefaultPrompt: ev.DefaultValue,
				})
			}

			// Respond off the event-bus goroutine to avoid deadlock.
			// Capture ctx here (inside the subscriber) so it reflects the
			// page ctx at dispatch time, honoring Close() cancellation.
			dialCtx := p.ctx()
			go func(a DialogAction, ctxID string) {
				accept := a.Accept
				_ = p.browser.client.HandleUserPrompt(dialCtx, bidi.HandleUserPromptParams{
					Context:  ctxID,
					Accept:   &accept,
					UserText: a.PromptText,
				})
			}(action, ev.Context)
		}))
	})
}

// OnConsole registers a handler invoked for every console.* call from the
// page. Replaces any previous handler — only one handler slot is kept.
// Pass nil to disable. Handler runs on a goroutine — do not block on
// firefoxkit calls into the same page from inside it.
//
// Subscription is created at most once per page; subsequent calls only replace
// the handler slot. Released automatically when the page is closed.
func (p *Page) OnConsole(handler func(ConsoleEvent)) {
	p.consoleHandler.Store(&handler)

	p.consoleOnce.Do(func() {
		if err := p.browser.client.SubscribeEvents(p.ctx(), bidi.SubscribeParams{
			Events:   []string{"log.entryAdded"},
			Contexts: []string{p.contextID},
		}); err != nil {
			if p.log != nil {
				p.log.Warnf("firefoxkit: OnConsole subscribe: %v", err)
			}
			// Reset Once so a future call can retry after a transient failure.
			p.consoleOnce = sync.Once{}
			return
		}
		p.trackSub(p.browser.client.Subscribe("log.entryAdded", func(params json.RawMessage) {
			var ev bidi.LogEntry
			if err := json.Unmarshal(params, &ev); err != nil {
				if p.log != nil {
					p.log.Warnf("firefoxkit: decode log.entryAdded: %v", err)
				}
				return
			}
			if ev.Source.Context != p.contextID {
				return
			}
			hp := p.consoleHandler.Load()
			if hp == nil || *hp == nil {
				return
			}
			go (*hp)(ConsoleEvent{
				Level:     ConsoleLevel(ev.Level),
				Text:      ev.Text,
				Method:    ev.Method,
				Timestamp: time.UnixMilli(ev.Timestamp),
			})
		}))
	})
}
