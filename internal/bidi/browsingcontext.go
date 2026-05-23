package bidi

import "context"

// BrowsingContextInfo is the per-context payload returned by getTree / create.
type BrowsingContextInfo struct {
	Context  string                `json:"context"`
	URL      string                `json:"url"`
	Children []BrowsingContextInfo `json:"children,omitempty"`
	Parent   string                `json:"parent,omitempty"`
	UserCtx  string                `json:"userContext,omitempty"`
}

// GetTreeResult is the response of browsingContext.getTree.
type GetTreeResult struct {
	Contexts []BrowsingContextInfo `json:"contexts"`
}

// GetTree returns the full browsing context tree. Top-level entries are tabs.
func (c *Client) GetTree(ctx context.Context) (*GetTreeResult, error) {
	var out GetTreeResult
	if err := c.Send(ctx, "browsingContext.getTree", struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateParams configures browsingContext.create.
type CreateParams struct {
	Type        string `json:"type"` // "tab" | "window"
	ReferenceID string `json:"referenceContext,omitempty"`
	Background  bool   `json:"background,omitempty"`
}

// CreateResult is the response of browsingContext.create.
type CreateResult struct {
	Context string `json:"context"`
}

// Create opens a new tab or window.
func (c *Client) Create(ctx context.Context, p CreateParams) (*CreateResult, error) {
	if p.Type == "" {
		p.Type = "tab"
	}
	var out CreateResult
	if err := c.Send(ctx, "browsingContext.create", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CloseParams configures browsingContext.close.
type CloseParams struct {
	Context     string `json:"context"`
	PromptUnload bool  `json:"promptUnload,omitempty"`
}

// CloseContext closes the given browsing context (tab).
func (c *Client) CloseContext(ctx context.Context, p CloseParams) error {
	return c.Send(ctx, "browsingContext.close", p, nil)
}

// NavigateWait is the BiDi readiness condition for navigate.
type NavigateWait string

const (
	WaitNone        NavigateWait = "none"
	WaitInteractive NavigateWait = "interactive"
	WaitComplete    NavigateWait = "complete"
)

// NavigateParams configures browsingContext.navigate.
type NavigateParams struct {
	Context string       `json:"context"`
	URL     string       `json:"url"`
	Wait    NavigateWait `json:"wait,omitempty"`
}

// NavigateResult is the response of browsingContext.navigate.
type NavigateResult struct {
	Navigation string `json:"navigation,omitempty"`
	URL        string `json:"url"`
}

// Navigate navigates the given context. wait defaults to "complete" if empty.
func (c *Client) Navigate(ctx context.Context, p NavigateParams) (*NavigateResult, error) {
	if p.Wait == "" {
		p.Wait = WaitComplete
	}
	var out NavigateResult
	if err := c.Send(ctx, "browsingContext.navigate", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReloadParams configures browsingContext.reload.
type ReloadParams struct {
	Context   string       `json:"context"`
	IgnoreCache bool       `json:"ignoreCache,omitempty"`
	Wait      NavigateWait `json:"wait,omitempty"`
}

// Reload reloads the given context.
func (c *Client) Reload(ctx context.Context, p ReloadParams) error {
	if p.Wait == "" {
		p.Wait = WaitComplete
	}
	return c.Send(ctx, "browsingContext.reload", p, nil)
}

// TraverseHistoryParams configures browsingContext.traverseHistory.
// Delta = -1 → back, +1 → forward.
type TraverseHistoryParams struct {
	Context string `json:"context"`
	Delta   int    `json:"delta"`
}

// TraverseHistory moves through history (back/forward).
func (c *Client) TraverseHistory(ctx context.Context, p TraverseHistoryParams) error {
	return c.Send(ctx, "browsingContext.traverseHistory", p, nil)
}

// ActivateParams configures browsingContext.activate (focus / bring to front).
type ActivateParams struct {
	Context string `json:"context"`
}

// Activate brings the given context to the foreground.
func (c *Client) Activate(ctx context.Context, p ActivateParams) error {
	return c.Send(ctx, "browsingContext.activate", p, nil)
}

// CaptureScreenshotParams configures browsingContext.captureScreenshot.
type CaptureScreenshotParams struct {
	Context string                  `json:"context"`
	Origin  string                  `json:"origin,omitempty"` // "viewport" | "document"
	Format  *ScreenshotFormat       `json:"format,omitempty"`
	Clip    *ScreenshotClip         `json:"clip,omitempty"`
}

// ScreenshotFormat selects the encoded image type.
type ScreenshotFormat struct {
	Type    string  `json:"type"`              // "image/png" | "image/jpeg"
	Quality float64 `json:"quality,omitempty"` // 0..1, JPEG only
}

// ScreenshotClip identifies the sub-rect to capture. Use BoxClip for
// pixel-rect clipping; ElementClip targets a node by handle.
type ScreenshotClip struct {
	Type   string  `json:"type"`             // "box" | "element"
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

// CaptureScreenshotResult is the response of browsingContext.captureScreenshot.
type CaptureScreenshotResult struct {
	Data string `json:"data"` // base64
}

// CaptureScreenshot returns a base64-encoded PNG (or JPEG) of the context.
func (c *Client) CaptureScreenshot(ctx context.Context, p CaptureScreenshotParams) (*CaptureScreenshotResult, error) {
	var out CaptureScreenshotResult
	if err := c.Send(ctx, "browsingContext.captureScreenshot", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HandleUserPromptParams configures browsingContext.handleUserPrompt.
type HandleUserPromptParams struct {
	Context string `json:"context"`
	Accept  *bool  `json:"accept,omitempty"`
	UserText string `json:"userText,omitempty"`
}

// HandleUserPrompt responds to an open dialog. Accept=true confirms;
// Accept=false dismisses. UserText fills prompt() input.
func (c *Client) HandleUserPrompt(ctx context.Context, p HandleUserPromptParams) error {
	return c.Send(ctx, "browsingContext.handleUserPrompt", p, nil)
}

// UserPromptOpenedEvent is the payload of browsingContext.userPromptOpened.
type UserPromptOpenedEvent struct {
	Context     string `json:"context"`
	Type        string `json:"type"` // "alert" | "confirm" | "prompt" | "beforeunload"
	Message     string `json:"message"`
	DefaultValue string `json:"defaultValue,omitempty"`
}
