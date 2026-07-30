package firefoxkit

import (
	"errors"

	"github.com/tuwibu/firefoxkit/input"
)

// ErrForegroundLost is raised when the browser window lost the OS foreground
// mid-input, so the remaining keystrokes would have landed somewhere else.
// Aliased from the input package: the sentinel has to live there (that is where
// it is returned from), and firefoxkit → firefoxkit/input is a one-way import.
var ErrForegroundLost = input.ErrForegroundLost

var (
	ErrContextCanceled      = errors.New("firefoxkit: context canceled")
	ErrPageNotFound         = errors.New("firefoxkit: page not found")
	ErrNavigate             = errors.New("firefoxkit: navigate failed")
	ErrTimeout              = errors.New("firefoxkit: timeout")
	ErrEvaluate             = errors.New("firefoxkit: evaluate failed")
	ErrNativeNotSupported   = errors.New("firefoxkit: native input not supported on this platform")
	ErrInvalidConfig        = errors.New("firefoxkit: invalid config")
	ErrFrameDetached        = errors.New("firefoxkit: frame detached")
	ErrDownloadCanceled     = errors.New("firefoxkit: download canceled")
	ErrTypeSelectorMismatch = errors.New("firefoxkit: type selector value mismatch")
	ErrTabActivateTimeout   = errors.New("firefoxkit: tab activation not confirmed (timeout)")
	ErrNoLaunchTarget       = errors.New("firefoxkit: no page target after launch")
	ErrCannotCloseLastPage  = errors.New("firefoxkit: cannot close the only page (use Close)")
	ErrBiDiSessionFailed    = errors.New("firefoxkit: bidi session establishment failed")
	ErrFirefoxNotReady      = errors.New("firefoxkit: firefox bidi endpoint not ready before deadline")
	ErrPermissionsRuntimeUnsupported = errors.New("firefoxkit: runtime per-origin permission grant is not supported; use WithPermissionsDefault at Launch")
)
