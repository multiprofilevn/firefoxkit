package firefoxkit

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Frame represents an iframe inside a Page. Methods delegate to BiDi using
// the frame's own browsingContext id.
type Frame struct {
	page      *Page
	contextID string
	parentID  string
	url       string
	name      string
}

// ID returns the BiDi browsingContext id of this frame.
func (f *Frame) ID() string {
	if f == nil {
		return ""
	}
	return f.contextID
}

// URL returns the frame's URL as last observed via getTree.
func (f *Frame) URL() string {
	if f == nil {
		return ""
	}
	return f.url
}

// Name returns the iframe element's `name` attribute.
func (f *Frame) Name() string {
	if f == nil {
		return ""
	}
	return f.name
}

// Evaluate runs JS inside this frame's realm.
func (f *Frame) Evaluate(ctx context.Context, expr string, out any) error {
	res, err := f.page.browser.client.Evaluate(ctx, bidi.EvaluateParams{
		Expression:   expr,
		Target:       bidi.EvaluateTarget{Context: f.contextID},
		AwaitPromise: true,
	})
	if err != nil {
		return wrapEvalErr(err)
	}
	return unmarshalEvalResult(res, out)
}

// Navigate drives the frame to url, waiting for `complete` readiness.
// Caller controls the deadline via ctx.
func (f *Frame) Navigate(ctx context.Context, url string) error {
	_, err := f.page.browser.client.Navigate(ctx, bidi.NavigateParams{
		Context: f.contextID,
		URL:     url,
		Wait:    bidi.WaitComplete,
	})
	return wrapNavErr(err)
}

// QuerySelector returns an ElementHandle for the first match inside the frame.
func (f *Frame) QuerySelector(ctx context.Context, sel string) (*ElementHandle, error) {
	fnSrc := `(sel) => document.querySelector(sel)`
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := f.page.browser.client.CallFunction(ctx, bidi.CallFunctionParams{
		FunctionDeclaration: fnSrc,
		AwaitPromise:        true,
		Target:              bidi.EvaluateTarget{Context: f.contextID},
		Arguments:           []json.RawMessage{arg},
		ResultOwnership:     bidi.OwnershipRoot,
	})
	if err != nil {
		return nil, wrapEvalErr(err)
	}
	var rv struct {
		Type   string `json:"type"`
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(res.Result, &rv); err != nil {
		return nil, wrapEvalErr(err)
	}
	if rv.Type == "null" || rv.Handle == "" {
		return nil, ErrPageNotFound
	}
	return &ElementHandle{page: f.page, handle: rv.Handle}, nil
}

// Frames returns the immediate child frames of this Page (one level deep).
// Use Page.AllFrames for a recursive walk.
func (p *Page) Frames(ctx context.Context) ([]*Frame, error) {
	info, err := p.lookupContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Frame, 0, len(info.Children))
	for i := range info.Children {
		c := &info.Children[i]
		out = append(out, &Frame{
			page: p, contextID: c.Context, parentID: info.Context, url: c.URL,
		})
	}
	return out, nil
}

// AllFrames returns every descendant frame, flattened.
func (p *Page) AllFrames(ctx context.Context) ([]*Frame, error) {
	info, err := p.lookupContext(ctx)
	if err != nil {
		return nil, err
	}
	var out []*Frame
	var walk func(parent *bidi.BrowsingContextInfo)
	walk = func(parent *bidi.BrowsingContextInfo) {
		for i := range parent.Children {
			c := &parent.Children[i]
			out = append(out, &Frame{
				page: p, contextID: c.Context, parentID: parent.Context, url: c.URL,
			})
			walk(c)
		}
	}
	walk(info)
	return out, nil
}

// MainFrame returns a Frame representing the top-level browsingContext —
// useful when a caller wants to use the Frame API uniformly.
func (p *Page) MainFrame() *Frame {
	return &Frame{page: p, contextID: p.contextID, url: p.URL()}
}

// FrameByURL returns the first descendant frame whose URL matches re.
func (p *Page) FrameByURL(ctx context.Context, re *regexp.Regexp) (*Frame, error) {
	frames, err := p.AllFrames(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range frames {
		if re.MatchString(f.url) {
			return f, nil
		}
	}
	return nil, ErrPageNotFound
}

// FrameByName returns the first descendant frame with the given `name` attr.
// Looking up by name needs a JS round-trip because the BiDi tree doesn't
// carry the iframe element's `name`.
func (p *Page) FrameByName(ctx context.Context, name string) (*Frame, error) {
	frames, err := p.AllFrames(ctx)
	if err != nil {
		return nil, err
	}
	// Walk parent's iframes to map name → contextID via window.frames.
	type iframeInfo struct {
		Name string `json:"name"`
		Src  string `json:"src"`
	}
	expr := `Array.from(document.querySelectorAll("iframe")).map(f => ({name: f.name || "", src: f.src || ""}))`
	var infos []iframeInfo
	if err := p.Evaluate(expr, &infos); err != nil {
		return nil, err
	}
	for _, info := range infos {
		if info.Name != name {
			continue
		}
		for _, f := range frames {
			if f.url == info.Src {
				f.name = name
				return f, nil
			}
		}
	}
	return nil, ErrPageNotFound
}
