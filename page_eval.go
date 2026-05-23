package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Evaluate runs expression as JS in this page's realm and decodes the result
// into out. Pass nil for out to discard the result.
func (p *Page) Evaluate(ctx context.Context, expression string, out any) error {
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
func (p *Page) EvalSelector(ctx context.Context, sel, fnBody string, out any) error {
	fnSrc := fmt.Sprintf(`(sel) => { const el = document.querySelector(sel); if (!el) return null; return (%s)(el); }`, fnBody)
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := p.browser.client.CallFunction(ctx, bidi.CallFunctionParams{
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
func (p *Page) EvalSelectorAll(ctx context.Context, sel, fnBody string, out any) error {
	fnSrc := fmt.Sprintf(`(sel) => { const els = document.querySelectorAll(sel); return (%s)(Array.from(els)); }`, fnBody)
	arg, _ := json.Marshal(map[string]any{"type": "string", "value": sel})
	res, err := p.browser.client.CallFunction(ctx, bidi.CallFunctionParams{
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
