package bidi

import "context"

// InterceptPhase enumerates the request lifecycle stages BiDi can pause at.
type InterceptPhase string

const (
	PhaseBeforeRequestSent InterceptPhase = "beforeRequestSent"
	PhaseResponseStarted   InterceptPhase = "responseStarted"
	PhaseAuthRequired      InterceptPhase = "authRequired"
)

// URLPattern is the discriminator for network URL matching. Use either
// String (full URL) or Pattern (split fields).
type URLPattern struct {
	Type     string `json:"type"`              // "string" | "pattern"
	URL      string `json:"url,omitempty"`     // for type=string
	Scheme   string `json:"scheme,omitempty"`  // for type=pattern
	Hostname string `json:"hostname,omitempty"`
	Port     string `json:"port,omitempty"`
	Pathname string `json:"pathname,omitempty"`
	Search   string `json:"search,omitempty"`
}

// AddInterceptParams configures network.addIntercept.
type AddInterceptParams struct {
	Phases      []InterceptPhase `json:"phases"`
	Contexts    []string         `json:"contexts,omitempty"`
	URLPatterns []URLPattern     `json:"urlPatterns,omitempty"`
}

// AddInterceptResult is the response of network.addIntercept.
type AddInterceptResult struct {
	Intercept string `json:"intercept"`
}

// AddIntercept registers a request interceptor and returns its id.
func (c *Client) AddIntercept(ctx context.Context, p AddInterceptParams) (*AddInterceptResult, error) {
	var out AddInterceptResult
	if err := c.Send(ctx, "network.addIntercept", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveInterceptParams configures network.removeIntercept.
type RemoveInterceptParams struct {
	Intercept string `json:"intercept"`
}

// RemoveIntercept releases a previously-added intercept.
func (c *Client) RemoveIntercept(ctx context.Context, p RemoveInterceptParams) error {
	return c.Send(ctx, "network.removeIntercept", p, nil)
}

// Header is one HTTP header in BiDi shape.
type Header struct {
	Name  string    `json:"name"`
	Value BiDiBytes `json:"value"`
}

// ContinueRequestParams configures network.continueRequest. Use to mutate
// (or pass through) an intercepted beforeRequestSent event.
type ContinueRequestParams struct {
	Request string    `json:"request"`
	Headers []Header  `json:"headers,omitempty"`
	Method  string    `json:"method,omitempty"`
	URL     string    `json:"url,omitempty"`
	Body    *BiDiBytes `json:"body,omitempty"`
}

// ContinueRequest releases an intercepted request.
func (c *Client) ContinueRequest(ctx context.Context, p ContinueRequestParams) error {
	return c.Send(ctx, "network.continueRequest", p, nil)
}

// BeforeRequestSentEvent is the payload of network.beforeRequestSent. We only
// capture the fields firefoxkit consumes — full BiDi schema has dozens more.
type BeforeRequestSentEvent struct {
	Context  string `json:"context"`
	Request  RequestData `json:"request"`
	Intercepts []string `json:"intercepts,omitempty"`
}

// RequestData is the per-request shape inside BeforeRequestSentEvent.
type RequestData struct {
	Request string   `json:"request"`
	URL     string   `json:"url"`
	Method  string   `json:"method"`
	Headers []Header `json:"headers"`
}

// DownloadEndEvent is the payload of browsingContext.downloadEnd.
type DownloadEndEvent struct {
	Context  string `json:"context"`
	URL      string `json:"url"`
	FilePath string `json:"filepath,omitempty"`
	Status   string `json:"status"` // "complete" | "canceled"
}
