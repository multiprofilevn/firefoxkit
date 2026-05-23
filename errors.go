package firefoxkit

import "errors"

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
