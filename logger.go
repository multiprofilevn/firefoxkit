package firefoxkit

import "github.com/multiprofilevn/firefoxkit/internal/bidi"

// Logger is the diagnostic interface firefoxkit uses. Inject via WithLogger;
// default is a no-op.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}

// bidiLoggerAdapter bridges firefoxkit.Logger -> bidi.Logger. Same shape,
// distinct types to keep the internal package free of root-package imports.
type bidiLoggerAdapter struct{ l Logger }

func (a bidiLoggerAdapter) Debugf(f string, args ...any) { a.l.Debugf(f, args...) }
func (a bidiLoggerAdapter) Infof(f string, args ...any)  { a.l.Infof(f, args...) }
func (a bidiLoggerAdapter) Warnf(f string, args ...any)  { a.l.Warnf(f, args...) }
func (a bidiLoggerAdapter) Errorf(f string, args ...any) { a.l.Errorf(f, args...) }

func toBiDiLogger(l Logger) bidi.Logger {
	if l == nil {
		return nil
	}
	return bidiLoggerAdapter{l: l}
}
