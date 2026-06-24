package firefoxkit

import "time"

// ConsoleLevel categorizes browser console output. Maps directly to BiDi's
// LogEntry.level field.
type ConsoleLevel string

const (
	ConsoleLog   ConsoleLevel = "log"
	ConsoleDebug ConsoleLevel = "debug"
	ConsoleInfo  ConsoleLevel = "info"
	ConsoleWarn  ConsoleLevel = "warn"
	ConsoleError ConsoleLevel = "error"
)

// ConsoleEvent is a single console.* entry. Level and Text match the
// chromekit ConsoleEvent shape for drop-in parity. Method and Timestamp
// are Firefox-specific extras retained for callers that need them.
type ConsoleEvent struct {
	Level     ConsoleLevel
	Text      string
	Method    string    // "log" | "info" | "warn" | "error" | "debug" | "trace" …
	Timestamp time.Time // wall time of the log entry
}
