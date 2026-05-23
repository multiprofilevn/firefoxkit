package bidi

import "context"

// PartitionDescriptor identifies a storage partition. Use Context for
// per-browsingContext storage; StorageKey for explicit storage key targeting.
type PartitionDescriptor struct {
	Type      string `json:"type"`       // "context" | "storageKey"
	Context   string `json:"context,omitempty"`
	UserContext string `json:"userContext,omitempty"`
	SourceOrigin string `json:"sourceOrigin,omitempty"`
}

// ContextPartition is a convenience builder for the common case.
func ContextPartition(contextID string) PartitionDescriptor {
	return PartitionDescriptor{Type: "context", Context: contextID}
}

// CookieFilter narrows a getCookies query. Empty fields match everything.
type CookieFilter struct {
	Name   string `json:"name,omitempty"`
	Value  *BiDiBytes `json:"value,omitempty"`
	Domain string `json:"domain,omitempty"`
	Path   string `json:"path,omitempty"`
	Secure *bool  `json:"secure,omitempty"`
	HTTPOnly *bool `json:"httpOnly,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

// BiDiBytes is the BiDi byte-value union { type:"string", value:"..." } |
// { type:"base64", value:"..." }.
type BiDiBytes struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// StringBytes is the common builder for textual cookie values.
func StringBytes(s string) *BiDiBytes { return &BiDiBytes{Type: "string", Value: s} }

// CookieInfo is one entry returned by getCookies.
type CookieInfo struct {
	Name     string    `json:"name"`
	Value    BiDiBytes `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Size     int       `json:"size"`
	HTTPOnly bool      `json:"httpOnly"`
	Secure   bool      `json:"secure"`
	SameSite string    `json:"sameSite"`
	Expiry   int64     `json:"expiry,omitempty"`
}

// GetCookiesParams configures storage.getCookies.
type GetCookiesParams struct {
	Filter    *CookieFilter       `json:"filter,omitempty"`
	Partition PartitionDescriptor `json:"partition"`
}

// GetCookiesResult is the response of storage.getCookies.
type GetCookiesResult struct {
	Cookies []CookieInfo `json:"cookies"`
	PartitionKey map[string]any `json:"partitionKey,omitempty"`
}

// GetCookies returns cookies matching the filter for the given partition.
func (c *Client) GetCookies(ctx context.Context, p GetCookiesParams) (*GetCookiesResult, error) {
	var out GetCookiesResult
	if err := c.Send(ctx, "storage.getCookies", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PartialCookie is the cookie shape accepted by storage.setCookie.
type PartialCookie struct {
	Name     string    `json:"name"`
	Value    BiDiBytes `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path,omitempty"`
	Secure   *bool     `json:"secure,omitempty"`
	HTTPOnly *bool     `json:"httpOnly,omitempty"`
	SameSite string    `json:"sameSite,omitempty"`
	Expiry   *int64    `json:"expiry,omitempty"`
}

// SetCookieParams configures storage.setCookie.
type SetCookieParams struct {
	Cookie    PartialCookie       `json:"cookie"`
	Partition PartitionDescriptor `json:"partition"`
}

// SetCookie sets a single cookie.
func (c *Client) SetCookie(ctx context.Context, p SetCookieParams) error {
	return c.Send(ctx, "storage.setCookie", p, nil)
}

// DeleteCookiesParams configures storage.deleteCookies.
type DeleteCookiesParams struct {
	Filter    *CookieFilter       `json:"filter,omitempty"`
	Partition PartitionDescriptor `json:"partition"`
}

// DeleteCookies deletes cookies matching the filter.
func (c *Client) DeleteCookies(ctx context.Context, p DeleteCookiesParams) error {
	return c.Send(ctx, "storage.deleteCookies", p, nil)
}
