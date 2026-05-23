package bidi

import "context"

// StatusResult is the response of session.status.
type StatusResult struct {
	Ready   bool   `json:"ready"`
	Message string `json:"message"`
}

// Status calls session.status — used to probe readiness before session.new.
func (c *Client) Status(ctx context.Context) (*StatusResult, error) {
	var out StatusResult
	if err := c.Send(ctx, "session.status", struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NewSessionParams configures session.new. Capabilities is intentionally
// passed as raw map — BiDi session capability shape evolves; tests can
// inspect what was sent without us forcing a strict struct here.
type NewSessionParams struct {
	Capabilities map[string]any `json:"capabilities"`
}

// NewSessionResult captures the bits we actually use downstream.
type NewSessionResult struct {
	SessionID    string         `json:"sessionId"`
	Capabilities map[string]any `json:"capabilities"`
}

// NewSession calls session.new.
func (c *Client) NewSession(ctx context.Context, p NewSessionParams) (*NewSessionResult, error) {
	if p.Capabilities == nil {
		p.Capabilities = map[string]any{"alwaysMatch": map[string]any{}}
	}
	var out NewSessionResult
	if err := c.Send(ctx, "session.new", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubscribeParams enables server-side event delivery for the given events.
// Contexts is optional — empty list = subscribe globally.
type SubscribeParams struct {
	Events   []string `json:"events"`
	Contexts []string `json:"contexts,omitempty"`
}

// Subscribe issues session.subscribe — tells the server which events to send.
// Combine with Client.Subscribe (which registers a local handler).
func (c *Client) SubscribeEvents(ctx context.Context, p SubscribeParams) error {
	return c.Send(ctx, "session.subscribe", p, nil)
}

// UnsubscribeEvents issues session.unsubscribe.
func (c *Client) UnsubscribeEvents(ctx context.Context, p SubscribeParams) error {
	return c.Send(ctx, "session.unsubscribe", p, nil)
}
