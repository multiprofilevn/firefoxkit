package firefoxkit

import (
	"encoding/json"
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

	p.listenerMu.Lock()
	defer p.listenerMu.Unlock()
	if p.cleanedUp.Load() {
		return // page already torn down — don't register an orphan subscription
	}
	if !p.dialogSubscribed {
		ctx := p.ctx() // capture before goroutine so Close() cancel is honoured
		if err := p.browser.client.SubscribeEvents(ctx, bidi.SubscribeParams{
			Events:   []string{"browsingContext.userPromptOpened"},
			Contexts: []string{p.contextID},
		}); err != nil {
			if p.log != nil {
				p.log.Warnf("firefoxkit: OnDialog subscribe: %v", err)
			}
			// Flag stays false so a future call can retry after a transient failure.
			return
		}
		p.dialogSubscribed = true
		p.trackServerEvent("browsingContext.userPromptOpened")
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

			// Subscription workers may issue RPCs directly — the bidi
			// dispatcher runs handlers off the reader goroutine.
			accept := action.Accept
			_ = p.browser.client.HandleUserPrompt(p.ctx(), bidi.HandleUserPromptParams{
				Context:  ev.Context,
				Accept:   &accept,
				UserText: action.PromptText,
			})
		}))
	}
}

// OnConsole registers a handler invoked for every console.* call from the
// page. Replaces any previous handler — only one handler slot is kept.
// Pass nil to disable. The handler runs on this subscription's worker
// goroutine (bounded queue): it may call firefoxkit APIs, but if it falls
// behind, newer console events are dropped rather than buffered unboundedly.
//
// Subscription is created at most once per page; subsequent calls only replace
// the handler slot. Released automatically when the page is closed.
func (p *Page) OnConsole(handler func(ConsoleEvent)) {
	p.consoleHandler.Store(&handler)

	p.listenerMu.Lock()
	defer p.listenerMu.Unlock()
	if p.cleanedUp.Load() {
		return // page already torn down — don't register an orphan subscription
	}
	if !p.consoleSubscribed {
		if err := p.browser.client.SubscribeEvents(p.ctx(), bidi.SubscribeParams{
			Events:   []string{"log.entryAdded"},
			Contexts: []string{p.contextID},
		}); err != nil {
			if p.log != nil {
				p.log.Warnf("firefoxkit: OnConsole subscribe: %v", err)
			}
			// Flag stays false so a future call can retry after a transient failure.
			return
		}
		p.consoleSubscribed = true
		p.trackServerEvent("log.entryAdded")
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
			// Runs on this subscription's worker — bounded queue, no
			// per-event goroutine. A slow handler drops only console events.
			(*hp)(ConsoleEvent{
				Level:     ConsoleLevel(ev.Level),
				Text:      ev.Text,
				Method:    ev.Method,
				Timestamp: time.UnixMilli(ev.Timestamp),
			})
		}))
	}
}
