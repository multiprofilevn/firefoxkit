package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Evaluate runs expression as JS in this page's realm and decodes the result
// into out. Pass nil for out to discard the result.
//
// Note: with no caller deadline the call falls under the client's default
// command backstop (60s unless changed via WithCommandTimeout). Scripts whose
// awaited promise legitimately outlives that should use EvaluateCtx with an
// explicit longer deadline.
func (p *Page) Evaluate(expression string, out any) error {
	return p.EvaluateCtx(p.ctx(), expression, out)
}

// EvaluateCtx is Evaluate with a caller-supplied context. A ctx WITH a
// deadline takes precedence over the default command backstop, so long JS
// awaits (> 60s) work by passing e.g. context.WithTimeout(ctx, 5*time.Minute).
func (p *Page) EvaluateCtx(ctx context.Context, expression string, out any) error {
	return p.evaluateCtx(ctx, expression, out)
}

// evaluateCtx is the internal ctx-aware variant used by poll loops (WaitFor*)
// that need to propagate a timeout-derived context into the JS call.
func (p *Page) evaluateCtx(ctx context.Context, expression string, out any) error {
	res, err := p.browser.client.Evaluate(ctx, bidi.EvaluateParams{
		Expression:   expression,
		Target:       bidi.EvaluateTarget{Context: p.contextID},
		AwaitPromise: true,
	})
	if err != nil {
		return wrapEvalErr(err)
	}
	return unmarshalEvalResult(res, out)
}

// EvalSelector runs (el) => fnBody against document.querySelector(sel). If
// the selector matches nothing, returns ErrPageNotFound.
func (p *Page) EvalSelector(sel, fnBody string, out any) error {
	fnSrc := fmt.Sprintf(`(sel) => { const el = document.querySelector(sel); if (!el) return null; return (%s)(el); }`, fnBody)
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := p.browser.client.CallFunction(p.ctx(), bidi.CallFunctionParams{
		FunctionDeclaration: fnSrc,
		AwaitPromise:        true,
		Target:              bidi.EvaluateTarget{Context: p.contextID},
		Arguments:           []json.RawMessage{arg},
	})
	if err != nil {
		return wrapEvalErr(err)
	}
	return unmarshalEvalResult(res, out)
}

// EvalSelectorAll runs (els) => fnBody against document.querySelectorAll(sel).
// fnBody receives the NodeList; cast to Array.from(...) inside if needed.
func (p *Page) EvalSelectorAll(sel, fnBody string, out any) error {
	fnSrc := fmt.Sprintf(`(sel) => { const els = document.querySelectorAll(sel); return (%s)(Array.from(els)); }`, fnBody)
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := p.browser.client.CallFunction(p.ctx(), bidi.CallFunctionParams{
		FunctionDeclaration: fnSrc,
		AwaitPromise:        true,
		Target:              bidi.EvaluateTarget{Context: p.contextID},
		Arguments:           []json.RawMessage{arg},
	})
	if err != nil {
		return wrapEvalErr(err)
	}
	return unmarshalEvalResult(res, out)
}
