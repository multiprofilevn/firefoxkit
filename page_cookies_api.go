package firefoxkit

import (
	"context"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Cookie is the firefoxkit-facing cookie shape. Mirrors chromekit naming.
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path,omitempty"`
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"httpOnly"`
	SameSite string    `json:"sameSite,omitempty"` // "strict" | "lax" | "none"
	Expires  time.Time `json:"expires,omitempty"`
}

// CookieAPI groups cookie methods for a Page.
type CookieAPI struct{ p *Page }

// Cookies returns the cookie namespace.
func (p *Page) Cookies() *CookieAPI { return &CookieAPI{p: p} }

// GetAll returns every cookie in the current browsing context's partition.
func (a *CookieAPI) GetAll(ctx context.Context) ([]Cookie, error) {
	res, err := a.p.browser.client.GetCookies(ctx, bidi.GetCookiesParams{
		Partition: bidi.ContextPartition(a.p.contextID),
	})
	if err != nil {
		return nil, err
	}
	return convertCookies(res.Cookies), nil
}

// Get returns the first cookie named name, or ErrPageNotFound if absent.
func (a *CookieAPI) Get(ctx context.Context, name string) (Cookie, error) {
	res, err := a.p.browser.client.GetCookies(ctx, bidi.GetCookiesParams{
		Filter:    &bidi.CookieFilter{Name: name},
		Partition: bidi.ContextPartition(a.p.contextID),
	})
	if err != nil {
		return Cookie{}, err
	}
	if len(res.Cookies) == 0 {
		return Cookie{}, ErrPageNotFound
	}
	return convertCookies(res.Cookies)[0], nil
}

// Set creates or updates a cookie. Domain is required by BiDi; if empty
// we resolve it from the current page URL host.
func (a *CookieAPI) Set(ctx context.Context, c Cookie) error {
	if c.Domain == "" {
		host, err := a.resolveHost(ctx)
		if err != nil {
			return err
		}
		c.Domain = host
	}
	var secure, httpOnly *bool
	if c.Secure {
		s := true
		secure = &s
	}
	if c.HTTPOnly {
		h := true
		httpOnly = &h
	}
	var expiry *int64
	if !c.Expires.IsZero() {
		e := c.Expires.Unix()
		expiry = &e
	}
	return a.p.browser.client.SetCookie(ctx, bidi.SetCookieParams{
		Cookie: bidi.PartialCookie{
			Name:     c.Name,
			Value:    *bidi.StringBytes(c.Value),
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   secure,
			HTTPOnly: httpOnly,
			SameSite: c.SameSite,
			Expiry:   expiry,
		},
		Partition: bidi.ContextPartition(a.p.contextID),
	})
}

// Delete removes the first cookie named name.
func (a *CookieAPI) Delete(ctx context.Context, name string) error {
	return a.p.browser.client.DeleteCookies(ctx, bidi.DeleteCookiesParams{
		Filter:    &bidi.CookieFilter{Name: name},
		Partition: bidi.ContextPartition(a.p.contextID),
	})
}

// Clear removes every cookie in the current partition.
func (a *CookieAPI) Clear(ctx context.Context) error {
	return a.p.browser.client.DeleteCookies(ctx, bidi.DeleteCookiesParams{
		Partition: bidi.ContextPartition(a.p.contextID),
	})
}

func (a *CookieAPI) resolveHost(ctx context.Context) (string, error) {
	var host string
	if err := a.p.Evaluate(ctx, `window.location.hostname`, &host); err != nil {
		return "", err
	}
	if host == "" {
		return "", ErrInvalidConfig
	}
	return host, nil
}

func convertCookies(in []bidi.CookieInfo) []Cookie {
	out := make([]Cookie, len(in))
	for i, c := range in {
		ck := Cookie{
			Name:     c.Name,
			Value:    c.Value.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
		}
		if c.Expiry > 0 {
			ck.Expires = time.Unix(c.Expiry, 0)
		}
		out[i] = ck
	}
	return out
}
