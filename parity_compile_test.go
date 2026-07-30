// parity_compile_test.go — compile-time assertions that firefoxkit's public
// API matches chromekit's signatures for every symbol listed in the phase-11
// spec. If any signature drifts, this file fails to compile.
//
// Pattern: method-value expressions  `(*T).Method` are assigned to typed vars.
// The assignment compiles only when the declared type exactly matches the
// method's actual signature. No runtime call is ever made.
package firefoxkit

import (
	"context"
	"time"

	"github.com/tuwibu/firefoxkit/input"
)

// ---------------------------------------------------------------------------
// Top-level constructors
// ---------------------------------------------------------------------------

var _ func(int, ...Option) (*Browser, error) = Connect
var _ func(string, ...Option) (*Browser, error) = Launch

// ---------------------------------------------------------------------------
// Browser methods
// ---------------------------------------------------------------------------

var (
	_ func(*Browser) (*Page, error)          = (*Browser).NewPage
	_ func(*Browser, string) error           = (*Browser).ClosePage
	_ func(*Browser, int) error              = (*Browser).SwitchPageByIndex
	_ func(*Browser, string) error           = (*Browser).SwitchPageByURL
	_ func(*Browser) error                   = (*Browser).SwitchPageByOrigin
	_ func(*Browser, context.Context) error  = (*Browser).Focus
	_ func(*Browser) error                   = (*Browser).GoBack
	_ func(*Browser) (int, int, error)       = (*Browser).ContentOffset
	_ func(*Browser) (float64, error)        = (*Browser).WindowScale
	_ func(*Browser) (input.Rect, error)     = (*Browser).WindowRect
	_ func(*Browser) error                   = (*Browser).InvalidateNativeWindow
	_ func(*Browser, uintptr)                = (*Browser).SetHWND
	_ func(*Browser) uint32                  = (*Browser).PID
	_ func(*Browser, string) error           = (*Browser).ClosePagesNotMatching
	_ func(*Browser, string, ...Permission) error = (*Browser).GrantPermissions
	_ func(*Browser) error                   = (*Browser).ResetPermissions
	_ func(*Browser) error                   = (*Browser).Close
	_ func(*Browser) *Page                   = (*Browser).Current
	_ func(*Browser) []*Page                 = (*Browser).Pages
	_ func(*Browser) BackendType             = (*Browser).InputBackend
	_ func(*Browser) <-chan struct{}          = (*Browser).Done
)

// ---------------------------------------------------------------------------
// Page methods — navigation + wait
// ---------------------------------------------------------------------------

var (
	_ func(*Page, string, time.Duration) error = (*Page).Navigate
	_ func(*Page, time.Duration) error         = (*Page).Reload
	_ func(*Page, time.Duration) error         = (*Page).GoBack
	_ func(*Page, time.Duration) error         = (*Page).GoForward
	_ func(*Page, string, time.Duration) error = (*Page).WaitForSelector
	_ func(*Page, time.Duration) error         = (*Page).WaitForReady
	_ func(*Page, string, time.Duration) error = (*Page).WaitForFunction
	_ func(*Page, time.Duration) error         = (*Page).WaitForChangeURL
)

// ---------------------------------------------------------------------------
// Page methods — JS evaluation + DOM
// ---------------------------------------------------------------------------

var (
	_ func(*Page, string, any) error               = (*Page).Evaluate
	_ func(*Page, string, string, any) error        = (*Page).EvalSelector
	_ func(*Page, string, time.Duration) (*ElementHandle, error) = (*Page).QuerySelector
	_ func(*Page, string) (string, error)           = (*Page).GetText
	_ func(*Page, string, string) (string, error)   = (*Page).GetAttribute
	_ func(*Page, string) error                     = (*Page).ScrollIntoView
)

// ---------------------------------------------------------------------------
// Page methods — input
// ---------------------------------------------------------------------------

var (
	_ func(*Page, string) error              = (*Page).Click
	_ func(*Page, string) error              = (*Page).ClickElement
	_ func(*Page, string, string) error      = (*Page).Type
	_ func(*Page, string, ...string) error   = (*Page).SetInputFiles
)

// ---------------------------------------------------------------------------
// Page methods — metadata
// ---------------------------------------------------------------------------

var (
	_ func(*Page) (string, error)  = (*Page).GetTitle
	_ func(*Page) (string, error)  = (*Page).GetURL
	_ func(*Page) (string, error)  = (*Page).GetOrigin
	_ func(*Page) error            = (*Page).BringToFront
	_ func(*Page) int              = (*Page).Index
)

// ---------------------------------------------------------------------------
// Page methods — screenshot
// ---------------------------------------------------------------------------

var (
	_ func(*Page) ([]byte, error)                         = (*Page).Screenshot
	_ func(*Page, float64, float64, float64, float64) ([]byte, error) = (*Page).ScreenshotClip
	_ func(*Page, string) ([]byte, error)                  = (*Page).ScreenshotElement
	_ func(*Page, int) ([]byte, error)                     = (*Page).FullScreenshot
)

// ---------------------------------------------------------------------------
// Page methods — headers + select
// ---------------------------------------------------------------------------

var (
	_ func(*Page, map[string]string) error = (*Page).SetExtraHeaders
	_ func(*Page, string, string) error    = (*Page).SetSelectValue
)

// ---------------------------------------------------------------------------
// Page methods — sub-namespaces
// ---------------------------------------------------------------------------

var (
	_ func(*Page) *CookiesAPI   = (*Page).Cookies
	_ func(*Page) *Storage      = (*Page).LocalStorage
	_ func(*Page) *Storage      = (*Page).SessionStorage
	_ func(*Page) *MouseAPI     = (*Page).Mouse
	_ func(*Page) *KeyboardAPI  = (*Page).Keyboard
	_ func(*Page) *DownloadsAPI = (*Page).Downloads
)

// ---------------------------------------------------------------------------
// Page methods — event handlers
// ---------------------------------------------------------------------------

var (
	_ func(*Page, func(ConsoleEvent))               = (*Page).OnConsole
	_ func(*Page, func(DialogEvent) DialogAction)   = (*Page).OnDialog
)

// ---------------------------------------------------------------------------
// MouseAPI
// ---------------------------------------------------------------------------

var (
	_ func(*MouseAPI, string) error            = (*MouseAPI).Click
	_ func(*MouseAPI, float64, float64) error  = (*MouseAPI).ClickAt
	_ func(*MouseAPI, string) error            = (*MouseAPI).DoubleClick
	_ func(*MouseAPI, string) error            = (*MouseAPI).RightClick
	_ func(*MouseAPI, string) error            = (*MouseAPI).Hover
	_ func(*MouseAPI, float64, float64) error  = (*MouseAPI).MoveTo
	_ func(*MouseAPI, float64, float64) error  = (*MouseAPI).Scroll
	_ func(*MouseAPI, float64, float64, float64, float64) error = (*MouseAPI).Drag
	_ func(*MouseAPI, string, string) error    = (*MouseAPI).DragAndDrop
	_ func(*MouseAPI, float64, float64) error  = (*MouseAPI).Tap
	_ func(*MouseAPI, string) error            = (*MouseAPI).FocusElement
)

// ---------------------------------------------------------------------------
// KeyboardAPI
// ---------------------------------------------------------------------------

var (
	_ func(*KeyboardAPI, string, ...input.Modifier) error           = (*KeyboardAPI).Press
	_ func(*KeyboardAPI, string, ...input.Modifier) error           = (*KeyboardAPI).Down
	_ func(*KeyboardAPI, string, ...input.Modifier) error           = (*KeyboardAPI).Up
	_ func(*KeyboardAPI, string) error                              = (*KeyboardAPI).Type
	_ func(*KeyboardAPI, string) error                              = (*KeyboardAPI).TypeFast
	_ func(*KeyboardAPI, string, ...input.HumanTypeConfig) error    = (*KeyboardAPI).TypeHuman
	_ func(*KeyboardAPI, string, string, ...TypeSelectorConfig) error = (*KeyboardAPI).TypeSelector
	_ func(*KeyboardAPI, ...string) error                           = (*KeyboardAPI).ClearInput
)

// ---------------------------------------------------------------------------
// CookiesAPI
// ---------------------------------------------------------------------------

var (
	_ func(*CookiesAPI, ...string) ([]Cookie, error) = (*CookiesAPI).Get
	_ func(*CookiesAPI, ...Cookie) error             = (*CookiesAPI).Set
	_ func(*CookiesAPI, string, string) error        = (*CookiesAPI).Delete
	_ func(*CookiesAPI) error                        = (*CookiesAPI).Clear
)

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

var (
	_ func(*Storage, string) (string, error) = (*Storage).Get
	_ func(*Storage, string, string) error   = (*Storage).Set
	_ func(*Storage, string) error           = (*Storage).Remove
	_ func(*Storage) error                   = (*Storage).Clear
)

// ---------------------------------------------------------------------------
// Permission constants (same string values as chromekit)
// ---------------------------------------------------------------------------

var _ = []Permission{
	PermissionGeolocation,
	PermissionCamera,
	PermissionMicrophone,
	PermissionNotifications,
	PermissionClipboardRead,
	PermissionClipboardWrite,
	PermissionMIDI,
	PermissionMIDISysex,
	PermissionBackgroundSync,
	PermissionPersistentStorage,
}

// ---------------------------------------------------------------------------
// Event types (shape parity with chromekit)
// ---------------------------------------------------------------------------

// ConsoleEvent: Level + Text fields match chromekit (Method/Timestamp are extras).
var _ = ConsoleEvent{Level: ConsoleLog, Text: ""}

// DialogEvent: Type + Message + DefaultPrompt match chromekit exactly.
var _ = DialogEvent{Type: DialogAlert, Message: "", DefaultPrompt: ""}

// DialogAction: Accept + PromptText match chromekit exactly.
var _ = DialogAction{Accept: false, PromptText: ""}

// ConsoleLevel constants match chromekit values.
var _ = []ConsoleLevel{ConsoleLog, ConsoleDebug, ConsoleInfo, ConsoleWarn, ConsoleError}

// DialogType constants match chromekit values.
var _ = []DialogType{DialogAlert, DialogConfirm, DialogPrompt, DialogBeforeUnload}

// BoundingBox — firefoxkit adds Center(); base fields X/Y/Width/Height present.
// Note: chromekit BoundingBox also has Left/Right/Top/Bottom — firefoxkit does
// not (BiDi only returns the basic rect). Documented exception.
var _ = BoundingBox{X: 0, Y: 0, Width: 0, Height: 0}
