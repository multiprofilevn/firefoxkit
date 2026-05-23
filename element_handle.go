package firefoxkit

import (
	"context"
	"encoding/json"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// ElementHandle wraps a BiDi remote-object handle pointing at a DOM node.
// Lives in the realm of its owning page; becomes stale on navigation or when
// the underlying node detaches.
type ElementHandle struct {
	page   *Page
	handle string // BiDi remote handle id (resultOwnership=root)
	sharedID string // optional shared id, useful for cross-realm refs
}

// Dispose releases the handle. Safe to call multiple times; later calls are
// no-ops. Always defer Dispose() after a Query that yields a handle the
// caller will not reuse — handles consume browser memory.
func (h *ElementHandle) Dispose(ctx context.Context) error {
	if h == nil || h.handle == "" {
		return nil
	}
	hid := h.handle
	h.handle = ""
	return h.page.browser.client.Disown(ctx, bidi.DisownParams{
		Handles: []string{hid},
		Target:  bidi.EvaluateTarget{Context: h.page.contextID},
	})
}

// EvaluateOn runs functionDeclaration with this handle as the implicit `this`
// argument and unmarshals the result into out. The function source must be a
// complete `(el) => ...` arrow expression — el is bound to this handle.
func (h *ElementHandle) EvaluateOn(ctx context.Context, fnSrc string, out any) error {
	arg, _ := json.Marshal(map[string]any{"handle": h.handle})
	res, err := h.page.browser.client.CallFunction(ctx, bidi.CallFunctionParams{
		FunctionDeclaration: fnSrc,
		AwaitPromise:        true,
		Target:              bidi.EvaluateTarget{Context: h.page.contextID},
		Arguments:           []json.RawMessage{arg},
	})
	if err != nil {
		return wrapEvalErr(err)
	}
	return unmarshalEvalResult(res, out)
}

// unmarshalEvalResult decodes RemoteValue → JSON → out. Centralized so every
// Evaluate-shaped caller behaves the same.
func unmarshalEvalResult(res *bidi.EvaluateResult, out any) error {
	if res.Type == "exception" {
		txt := ""
		if res.ExceptionDetails != nil {
			txt = res.ExceptionDetails.Text
		}
		return wrapEvalErr(&bidi.BiDiError{Code: "exception", Message: txt})
	}
	if out == nil || len(res.Result) == 0 {
		return nil
	}
	decoded, err := bidi.DecodeRemoteValue(res.Result)
	if err != nil {
		return wrapEvalErr(err)
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return wrapEvalErr(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return wrapEvalErr(err)
	}
	return nil
}

func wrapEvalErr(err error) error {
	if err == nil {
		return nil
	}
	return &evalError{cause: err}
}

type evalError struct{ cause error }

func (e *evalError) Error() string { return "firefoxkit: evaluate: " + e.cause.Error() }
func (e *evalError) Unwrap() error { return e.cause }
func (e *evalError) Is(target error) bool {
	return target == ErrEvaluate
}
