package firefoxkit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// DialogHandler is invoked when Firefox raises a user prompt. The handler
// MUST call Accept or Dismiss; otherwise the page hangs.
type DialogHandler func(*Dialog)

// ConsoleHandler receives console.* output from the page.
type ConsoleHandler func(ConsoleMessage)

// OnDialog registers a handler for userPromptOpened events scoped to this page.
// Returns a cancellation function — calling it (or Browser.ClosePage on the
// owning Page) unsubscribes the handler.
//
// ctx is used for the subscribe RPC only; handler invocations happen later
// on the event-bus goroutine and do NOT inherit this ctx.
func (p *Page) OnDialog(ctx context.Context, handler DialogHandler) (cancel func(), err error) {
	if err := p.browser.client.SubscribeEvents(ctx, bidi.SubscribeParams{
		Events:   []string{"browsingContext.userPromptOpened"},
		Contexts: []string{p.contextID},
	}); err != nil {
		return nil, err
	}
	sub := p.trackSub(p.browser.client.Subscribe("browsingContext.userPromptOpened", func(params json.RawMessage) {
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
		handler(&Dialog{
			page:         p,
			contextID:    ev.Context,
			Type:         DialogType(ev.Type),
			Message:      ev.Message,
			DefaultValue: ev.DefaultValue,
		})
	}))
	return func() { sub.Unsubscribe() }, nil
}

// OnConsole registers a handler for console output from this page.
// Returns a cancellation function. Browser.ClosePage on the owning Page
// also unsubscribes — see Page.cleanup.
func (p *Page) OnConsole(ctx context.Context, handler ConsoleHandler) (cancel func(), err error) {
	if err := p.browser.client.SubscribeEvents(ctx, bidi.SubscribeParams{
		Events:   []string{"log.entryAdded"},
		Contexts: []string{p.contextID},
	}); err != nil {
		return nil, err
	}
	sub := p.trackSub(p.browser.client.Subscribe("log.entryAdded", func(params json.RawMessage) {
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
		handler(ConsoleMessage{
			Level:     ConsoleLevel(ev.Level),
			Text:      ev.Text,
			Method:    ev.Method,
			Timestamp: time.UnixMilli(ev.Timestamp),
		})
	}))
	return func() { sub.Unsubscribe() }, nil
}
