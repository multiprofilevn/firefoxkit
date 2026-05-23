package firefoxkit

import "time"

// ConsoleLevel categorizes browser console output. Maps directly to BiDi's
// LogEntry.level field.
type ConsoleLevel string

const (
	ConsoleDebug ConsoleLevel = "debug"
	ConsoleInfo  ConsoleLevel = "info"
	ConsoleWarn  ConsoleLevel = "warn"
	ConsoleError ConsoleLevel = "error"
)

// ConsoleMessage is a single console.* entry.
type ConsoleMessage struct {
	Level     ConsoleLevel
	Text      string
	Method    string // "log" | "info" | "warn" | "error" | "debug" | "trace" ...
	Timestamp time.Time
}
