package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// BoundingBox holds the rectangle returned by getBoundingClientRect.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Center returns the midpoint of the box.
func (b BoundingBox) Center() (float64, float64) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// QuerySelector returns an ElementHandle for the first match, or ErrPageNotFound
// if no element matches.
func (p *Page) QuerySelector(ctx context.Context, sel string) (*ElementHandle, error) {
	fnSrc := `(sel) => document.querySelector(sel)`
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := p.browser.client.CallFunction(ctx, bidi.CallFunctionParams{
		FunctionDeclaration: fnSrc,
		AwaitPromise:        true,
		Target:              bidi.EvaluateTarget{Context: p.contextID},
		Arguments:           []json.RawMessage{arg},
		ResultOwnership:     bidi.OwnershipRoot,
	})
	if err != nil {
		return nil, wrapEvalErr(err)
	}
	if res.Type == "exception" {
		return nil, wrapEvalErr(fmt.Errorf("querySelector: %s", res.ExceptionDetails.Text))
	}
	// Result is a RemoteValue. type=node + handle present, OR type=null.
	var rv struct {
		Type     string `json:"type"`
		Handle   string `json:"handle"`
		SharedID string `json:"sharedId"`
	}
	if err := json.Unmarshal(res.Result, &rv); err != nil {
		return nil, wrapEvalErr(err)
	}
	if rv.Type == "null" || rv.Handle == "" {
		return nil, ErrPageNotFound
	}
	return &ElementHandle{page: p, handle: rv.Handle, sharedID: rv.SharedID}, nil
}

// QuerySelectorAll returns ElementHandles for every match. Empty slice if none.
// Caller MUST Dispose() each handle when done.
func (p *Page) QuerySelectorAll(ctx context.Context, sel string) ([]*ElementHandle, error) {
	fnSrc := `(sel) => Array.from(document.querySelectorAll(sel))`
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := p.browser.client.CallFunction(ctx, bidi.CallFunctionParams{
		FunctionDeclaration: fnSrc,
		AwaitPromise:        true,
		Target:              bidi.EvaluateTarget{Context: p.contextID},
		Arguments:           []json.RawMessage{arg},
		ResultOwnership:     bidi.OwnershipRoot,
	})
	if err != nil {
		return nil, wrapEvalErr(err)
	}
	var rv struct {
		Type  string            `json:"type"`
		Value []json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(res.Result, &rv); err != nil {
		return nil, wrapEvalErr(err)
	}
	out := make([]*ElementHandle, 0, len(rv.Value))
	for _, item := range rv.Value {
		var n struct {
			Type   string `json:"type"`
			Handle string `json:"handle"`
		}
		if err := json.Unmarshal(item, &n); err != nil {
			continue
		}
		if n.Handle != "" {
			out = append(out, &ElementHandle{page: p, handle: n.Handle})
		}
	}
	return out, nil
}

// BoundingBox returns the layout rectangle of the first matching element.
func (p *Page) BoundingBox(ctx context.Context, sel string) (BoundingBox, error) {
	var bb BoundingBox
	err := p.EvalSelector(ctx, sel, `(el) => { const r = el.getBoundingClientRect(); return {x:r.x, y:r.y, width:r.width, height:r.height}; }`, &bb)
	if err != nil {
		return bb, err
	}
	if bb.Width == 0 && bb.Height == 0 {
		// Element exists but is collapsed — still return the box; caller decides.
	}
	return bb, nil
}

// GetText returns the textContent of the first matching element.
func (p *Page) GetText(ctx context.Context, sel string) (string, error) {
	var s string
	if err := p.EvalSelector(ctx, sel, `(el) => el.textContent || ""`, &s); err != nil {
		return "", err
	}
	return s, nil
}

// GetAttribute returns the named attribute of the first matching element.
// Empty string if absent.
func (p *Page) GetAttribute(ctx context.Context, sel, name string) (string, error) {
	expr := fmt.Sprintf(`(el) => el.getAttribute(%q) || ""`, name)
	var s string
	if err := p.EvalSelector(ctx, sel, expr, &s); err != nil {
		return "", err
	}
	return s, nil
}

// ScrollIntoView scrolls the first matching element into view via
// scrollIntoView({block:"center"}).
func (p *Page) ScrollIntoView(ctx context.Context, sel string) error {
	return p.EvalSelector(ctx, sel, `(el) => { el.scrollIntoView({block:"center", inline:"center", behavior:"instant"}); return true; }`, nil)
}

// Focus runs el.focus() on the first matching element.
func (p *Page) Focus(ctx context.Context, sel string) error {
	return p.EvalSelector(ctx, sel, `(el) => { el.focus(); return true; }`, nil)
}
