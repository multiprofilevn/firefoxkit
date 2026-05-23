package bidi

import "encoding/json"

// frame is the raw on-wire envelope. BiDi has three shapes:
//   - command:  {id, method, params}                (client -> browser)
//   - success:  {id, type:"success", result}        (browser -> client)
//   - error:    {id, type:"error", error, message, stacktrace}
//   - event:    {type:"event", method, params}      (browser -> client, no id)
//
// We discriminate on which fields are present rather than on `type` alone —
// older FF builds omit `type` for plain command responses.
type frame struct {
	ID         *int            `json:"id,omitempty"`
	Type       string          `json:"type,omitempty"`
	Method     string          `json:"method,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Message    string          `json:"message,omitempty"`
	Stacktrace string          `json:"stacktrace,omitempty"`
}

// isResponse reports whether the frame is a response to a command we sent.
func (f *frame) isResponse() bool { return f.ID != nil }

// isError reports whether the frame is a BiDi error response. Only valid
// when isResponse() is true.
func (f *frame) isError() bool { return f.Type == "error" || f.Error != "" }

// isEvent reports whether the frame is an event (no id, has method).
func (f *frame) isEvent() bool { return f.ID == nil && f.Method != "" }

// command is the client->browser frame builder.
type command struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}
