package bidi

import (
	"context"
	"encoding/json"
	"fmt"
)

// ResultOwnership controls whether script.evaluate returns a handle the
// client must explicitly disown ("root") or no handle ("none", default).
type ResultOwnership string

const (
	OwnershipNone ResultOwnership = "none"
	OwnershipRoot ResultOwnership = "root"
)

// EvaluateTarget identifies where to run the expression. Use Context for
// per-browsingContext eval; Realm for explicit realm targeting.
type EvaluateTarget struct {
	Context string `json:"context,omitempty"`
	Realm   string `json:"realm,omitempty"`
	Sandbox string `json:"sandbox,omitempty"`
}

// EvaluateParams configures script.evaluate.
type EvaluateParams struct {
	Expression      string          `json:"expression"`
	Target          EvaluateTarget  `json:"target"`
	AwaitPromise    bool            `json:"awaitPromise"`
	ResultOwnership ResultOwnership `json:"resultOwnership,omitempty"`
	UserActivation  bool            `json:"userActivation,omitempty"`
}

// EvaluateResult is the response of script.evaluate.
type EvaluateResult struct {
	Type             string            `json:"type"` // "success" | "exception"
	Realm            string            `json:"realm"`
	Result           json.RawMessage   `json:"result,omitempty"`
	ExceptionDetails *ExceptionDetails `json:"exceptionDetails,omitempty"`
}

// ExceptionDetails is the BiDi shape for thrown errors.
type ExceptionDetails struct {
	Text        string          `json:"text"`
	LineNumber  int             `json:"lineNumber"`
	Column      int             `json:"columnNumber"`
	Exception   json.RawMessage `json:"exception"`
	StackTrace  json.RawMessage `json:"stackTrace,omitempty"`
}

// Evaluate runs expression in the given target and returns the result.
func (c *Client) Evaluate(ctx context.Context, p EvaluateParams) (*EvaluateResult, error) {
	var out EvaluateResult
	if err := c.Send(ctx, "script.evaluate", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CallFunctionParams configures script.callFunction.
type CallFunctionParams struct {
	FunctionDeclaration string             `json:"functionDeclaration"`
	AwaitPromise        bool               `json:"awaitPromise"`
	Target              EvaluateTarget     `json:"target"`
	Arguments           []json.RawMessage  `json:"arguments,omitempty"`
	This                json.RawMessage    `json:"this,omitempty"`
	ResultOwnership     ResultOwnership    `json:"resultOwnership,omitempty"`
}

// CallFunction invokes a function declaration against the target context.
// The function source is a complete `(arg0, arg1, ...) => { ... }` literal.
func (c *Client) CallFunction(ctx context.Context, p CallFunctionParams) (*EvaluateResult, error) {
	var out EvaluateResult
	if err := c.Send(ctx, "script.callFunction", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DisownParams configures script.disown.
type DisownParams struct {
	Handles []string       `json:"handles"`
	Target  EvaluateTarget `json:"target"`
}

// Disown releases the given remote-object handles.
func (c *Client) Disown(ctx context.Context, p DisownParams) error {
	return c.Send(ctx, "script.disown", p, nil)
}

// --- RemoteValue decoding ---------------------------------------------------

// remoteValueShape captures the discriminator + payload fields. We unmarshal
// once into this, then walk to a plain Go value (any).
type remoteValueShape struct {
	Type   string          `json:"type"`
	Value  json.RawMessage `json:"value,omitempty"`
	Handle string          `json:"handle,omitempty"`
}

// DecodeRemoteValue converts a BiDi RemoteValue JSON blob into a plain Go
// value (map[string]any / []any / primitive). The output is JSON-shaped so
// callers can json.Marshal it again and Unmarshal into a typed struct.
//
// Primitive types map to their Go counterparts. Objects expressed as
// [["k", RemoteValue], ...] flatten to map[string]any. Arrays flatten to []any.
// Unknown / opaque types (function, node, weakref, ...) become a tag map
// {"$bidi": <type>, "handle": "..."} so callers can still introspect.
func DecodeRemoteValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rv remoteValueShape
	if err := json.Unmarshal(raw, &rv); err != nil {
		return nil, fmt.Errorf("bidi: decode RemoteValue: %w", err)
	}
	switch rv.Type {
	case "undefined", "null":
		return nil, nil
	case "string":
		var s string
		_ = json.Unmarshal(rv.Value, &s)
		return s, nil
	case "number":
		// May be `42`, `"NaN"`, `"-Infinity"`, etc.
		var f float64
		if err := json.Unmarshal(rv.Value, &f); err == nil {
			return f, nil
		}
		var s string
		_ = json.Unmarshal(rv.Value, &s)
		return s, nil
	case "boolean":
		var b bool
		_ = json.Unmarshal(rv.Value, &b)
		return b, nil
	case "bigint":
		var s string
		_ = json.Unmarshal(rv.Value, &s)
		return s, nil
	case "array", "set":
		var elems []json.RawMessage
		if err := json.Unmarshal(rv.Value, &elems); err != nil {
			return nil, err
		}
		out := make([]any, len(elems))
		for i, e := range elems {
			v, err := DecodeRemoteValue(e)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case "object", "map":
		var pairs [][2]json.RawMessage
		if err := json.Unmarshal(rv.Value, &pairs); err != nil {
			// Some BiDi servers send object as map; tolerate both.
			var m map[string]json.RawMessage
			if jerr := json.Unmarshal(rv.Value, &m); jerr == nil {
				out := make(map[string]any, len(m))
				for k, v := range m {
					dv, e := DecodeRemoteValue(v)
					if e != nil {
						return nil, e
					}
					out[k] = dv
				}
				return out, nil
			}
			return nil, err
		}
		out := make(map[string]any, len(pairs))
		for _, p := range pairs {
			var k string
			if err := json.Unmarshal(p[0], &k); err != nil {
				// Key may itself be a RemoteValue; decode + stringify.
				kv, err := DecodeRemoteValue(p[0])
				if err != nil {
					return nil, err
				}
				k = fmt.Sprintf("%v", kv)
			}
			v, err := DecodeRemoteValue(p[1])
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	default:
		// node / function / weakref / regexp / date / ... — keep handle.
		return map[string]any{"$bidi": rv.Type, "handle": rv.Handle}, nil
	}
}
