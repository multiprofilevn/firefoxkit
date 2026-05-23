package bidi

import "encoding/json"

// LogEntry is the payload of log.entryAdded events.
type LogEntry struct {
	Type     string          `json:"type"` // "console" | "javascript"
	Level    string          `json:"level"` // "debug"|"info"|"warn"|"error"
	Source   LogSource       `json:"source"`
	Text     string          `json:"text"`
	Timestamp int64          `json:"timestamp"`
	Method   string          `json:"method,omitempty"`
	Args     []json.RawMessage `json:"args,omitempty"`
	StackTrace json.RawMessage `json:"stackTrace,omitempty"`
}

// LogSource identifies the originating realm + context for a log entry.
type LogSource struct {
	Realm   string `json:"realm,omitempty"`
	Context string `json:"context,omitempty"`
}
