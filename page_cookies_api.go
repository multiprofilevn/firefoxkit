package firefoxkit

import (
	"net/url"
	"strings"
	"time"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// Cookie is the firefoxkit-facing cookie shape. Mirrors chromekit naming.
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path,omitempty"`
	URL      string    `json:"url,omitempty"`      // when set, infers Domain/Path from this URL
	Expires  time.Time `json:"expires,omitempty"`
	HTTPOnly bool      `json:"httpOnly"`
	Secure   bool      `json:"secure"`
	SameSite string    `json:"sameSite,omitempty"` // "Strict" | "Lax" | "None" — empty means default
}

// CookiesAPI groups cookie methods for a Page.
type CookiesAPI struct{ p *Page }

// Cookies returns the cookie namespace.
func (p *Page) Cookies() *CookiesAPI { return &CookiesAPI{p: p} }

// Get returns cookies visible to the current page. Pass urls to scope the
// query to specific URLs; empty → all cookies in the current context partition.
func (a *CookiesAPI) Get(urls ...string) ([]Cookie, error) {
	ctx := a.p.ctx()
	if len(urls) == 0 {
		res, err := a.p.browser.client.GetCookies(ctx, bidi.GetCookiesParams{
			Partition: bidi.ContextPartition(a.p.contextID),
		})
		if err != nil {
			return nil, err
		}
		return convertCookies(res.Cookies), nil
	}
	// Filter by URL: query each URL and merge (deduplicate by name+domain+path).
	seen := map[string]struct{}{}
	var out []Cookie
	for _, u := range urls {
		res, err := a.p.browser.client.GetCookies(ctx, bidi.GetCookiesParams{
			Filter:    &bidi.CookieFilter{Domain: domainFromURL(u)},
			Partition: bidi.ContextPartition(a.p.contextID),
		})
		if err != nil {
			return nil, err
		}
		for _, c := range convertCookies(res.Cookies) {
			key := c.Name + "\x00" + c.Domain + "\x00" + c.Path
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// Set creates or updates one or more cookies. Each cookie must have either URL
// (preferred — BiDi infers Domain/Path) or Domain set; missing both resolves
// to the current page's host. SameSite input is case-insensitive ("lax"/"Lax"
// both accepted).
func (a *CookiesAPI) Set(cookies ...Cookie) error {
	ctx := a.p.ctx()
	for _, c := range cookies {
		if c.Domain == "" && c.URL == "" {
			host, err := a.resolveHost()
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
		// Normalize SameSite: accept "lax"/"Lax"/"LAX" → canonical BiDi value.
		sameSite := normalizeSameSite(c.SameSite)
		if err := a.p.browser.client.SetCookie(ctx, bidi.SetCookieParams{
			Cookie: bidi.PartialCookie{
				Name:     c.Name,
				Value:    *bidi.StringBytes(c.Value),
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   secure,
				HTTPOnly: httpOnly,
				SameSite: sameSite,
				Expiry:   expiry,
			},
			Partition: bidi.ContextPartition(a.p.contextID),
		}); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a single cookie by name. cookieURL scopes the deletion to a
// matching domain/path (recommended); pass empty string to delete by name only.
func (a *CookiesAPI) Delete(name, cookieURL string) error {
	ctx := a.p.ctx()
	filter := &bidi.CookieFilter{Name: name}
	if cookieURL != "" {
		filter.Domain = domainFromURL(cookieURL)
	}
	return a.p.browser.client.DeleteCookies(ctx, bidi.DeleteCookiesParams{
		Filter:    filter,
		Partition: bidi.ContextPartition(a.p.contextID),
	})
}

// Clear removes every cookie in the current partition.
func (a *CookiesAPI) Clear() error {
	ctx := a.p.ctx()
	return a.p.browser.client.DeleteCookies(ctx, bidi.DeleteCookiesParams{
		Partition: bidi.ContextPartition(a.p.contextID),
	})
}

func (a *CookiesAPI) resolveHost() (string, error) {
	var host string
	if err := a.p.Evaluate(`window.location.hostname`, &host); err != nil {
		return "", err
	}
	if host == "" {
		return "", ErrInvalidConfig
	}
	return host, nil
}

// normalizeSameSite accepts case-insensitive input ("lax", "Lax", "LAX") and
// returns the canonical BiDi value ("lax"). Empty string passes through.
func normalizeSameSite(s string) string {
	switch strings.ToLower(s) {
	case "strict":
		return "strict"
	case "lax":
		return "lax"
	case "none":
		return "none"
	default:
		return s
	}
}

// domainFromURL extracts the hostname (without port) from a URL string for use
// as a cookie domain filter. Uses net/url so port-suffixed hosts like
// "localhost:3000" and IPv6 addresses "[::1]" are handled correctly.
// Returns rawURL unchanged on parse failure (BiDi treats unknown domains as
// no-filter, which is safe).
func domainFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return rawURL
	}
	return u.Hostname()
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
