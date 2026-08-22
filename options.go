package firefoxkit

import (
	"os/exec"
	"strconv"
	"time"

	"github.com/multiprofilevn/firefoxkit/input"
)

// BackendType selects how firefoxkit dispatches mouse/keyboard input.
type BackendType int

const (
	// BackendBiDi dispatches input via BiDi input.performActions. Works on
	// every platform, even when the window is not focused.
	BackendBiDi BackendType = iota
	// BackendNative drives the OS pointer/keyboard directly (winapi on Windows).
	// Only works when the Firefox window is focused and visible.
	BackendNative
)

type nativeWindow struct {
	X, Y  int
	Scale float64
}

// CursorProfile selects the cursor movement style used by the native backend.
type CursorProfile int

const (
	CursorHuman CursorProfile = iota
	CursorFast
	CursorMachine
)

type config struct {
	logger        Logger
	userDataDir   string
	headless      bool
	proxy         string
	inputBackend  BackendType
	nativeWindow  nativeWindow
	cursorProfile CursorProfile
	cursorTuning  input.CursorTuning
	mousePathURL  string
	flags         map[string]string
	prefs         map[string]any
	modifyCmd      func(*exec.Cmd)
	launchTimeout  time.Duration
	commandTimeout time.Duration
	writeTimeout   time.Duration

	foregroundGuard bool
	// contentOffsetTolerance bounds measurements taken inside the page realm.
	// See WithContentOffsetTolerance.
	contentOffsetTolerance int
}

// DefaultContentOffsetTolerance is the slack, in screen pixels, allowed on a
// content-origin measurement taken inside the page realm. Borders and DPI
// rounding account for a couple of pixels; anything larger is a measurement
// that cannot be trusted.
const DefaultContentOffsetTolerance = 4

func defaultConfig() *config {
	return &config{
		logger:        noopLogger{},
		headless:      false,
		inputBackend:  BackendBiDi,
		nativeWindow:  nativeWindow{Scale: 1.0},
		cursorProfile: CursorHuman,
		flags:         map[string]string{},
		prefs:         map[string]any{},
		launchTimeout: 30 * time.Second,
		// On by default: typing into the wrong window is destructive, and the
		// check costs one syscall per five characters.
		foregroundGuard: true,

		contentOffsetTolerance: DefaultContentOffsetTolerance,
		// Backstop for BiDi calls whose ctx carries no deadline — wide enough
		// for the slowest legitimate RPC (screenshot/nav ~ seconds).
		commandTimeout: 60 * time.Second,
	}
}

// Option configures Launch / Connect.
type Option func(*config)

func WithLogger(l Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

func WithUserDataDir(dir string) Option {
	return func(c *config) { c.userDataDir = dir }
}

func WithHeadless(h bool) Option {
	return func(c *config) { c.headless = h }
}

// WithProxy injects proxy settings as Firefox prefs (network.proxy.*).
// addr format: "host:port" — uses HTTP proxy for HTTP and HTTPS.
func WithProxy(addr string) Option {
	return func(c *config) { c.proxy = addr }
}

func WithInputBackend(t BackendType) Option {
	return func(c *config) { c.inputBackend = t }
}

// WithForegroundGuard toggles the native backend's foreground check. When on
// (the default), every keystroke chunk and mouse button event confirms the
// Firefox window still owns the OS foreground and fails with ErrForegroundLost
// if it does not — SendInput has no window address, so input from an
// unfocused browser silently types into whatever took focus.
//
// No-op for the BiDi backend, which addresses the window directly and does not
// need focus at all. Turn it off only when something else already holds a
// foreground lease for the whole critical section.
func WithForegroundGuard(on bool) Option {
	return func(c *config) { c.foregroundGuard = on }
}

// WithMousePathURL sets the full URL of a mouse-positions endpoint that returns
// a high-entropy {x,y,delay} cursor path. When set, ProfileHuman moves fetch
// from it and replay; empty (default) keeps the pure local bezier engine.
func WithMousePathURL(url string) Option {
	return func(c *config) { c.mousePathURL = url }
}

// WithNativeWindow is a MANUAL OVERRIDE for the coordinate frame, used only
// when the live window cannot be measured — no HWND resolves (the Connect path
// before SetHWND), or the Win32 client origin is unavailable.
//
// The normal path does not need it: the kit measures the content origin and the
// DPI scale from the live window on every frame pin, and those measurements win
// over anything set here. Set it when a browser is driven natively on a host
// where the HWND lookup cannot succeed and the window position is known up
// front instead.
func WithNativeWindow(x, y int, scale float64) Option {
	return func(c *config) {
		if scale <= 0 {
			scale = 1.0
		}
		c.nativeWindow = nativeWindow{X: x, Y: y, Scale: scale}
	}
}

// WithContentOffsetTolerance sets the slack, in screen pixels, allowed on a
// measurement taken inside the page realm before it is discarded in favour of a
// trusted default.
//
// Firefox's content origin is based on GetClientRect + ClientToScreen, which no
// page can influence; only the chrome-area height on top of it is measured in
// the page, and this is the tolerance applied to it. chromekit — forced to
// measure the whole origin in the page, because Chrome paints its chrome inside
// the Win32 client area — applies the same option to the whole origin.
//
// px <= 0 is ignored. Default 4.
func WithContentOffsetTolerance(px int) Option {
	return func(c *config) {
		if px <= 0 {
			return
		}
		c.contentOffsetTolerance = px
	}
}

// prefDevPixelsPerPx is Firefox's device-pixel ratio override — a CHAR pref, so
// the value is written as a string ("1.5"). Firefox's own default is "-1.0" =
// follow system DPI, which is what an unset pref means here.
const prefDevPixelsPerPx = "layout.css.devPixelsPerPx"

// WithScaleFactor forces the profile's device-pixel ratio (Chrome's
// --force-device-scale-factor / --scale-factor equivalent). f <= 0 is ignored,
// leaving Firefox on system DPI. Applied per profile at launch via user.js —
// it cannot be changed at runtime.
//
// Interacts with the two native-window APIs, all three needed for the tiled
// multi-window native setup:
//   - WithNativeWindow(x, y, scale) — the scale the native backend uses to map
//     CSS coords to screen pixels. Set it to the SAME value passed here when
//     the live HWND measurement is unavailable, or clicks miss by that ratio.
//     When measurement succeeds the real DPI scale wins over both.
//   - Browser.SetWindowBounds — moves/resizes the window for tiling. Bounds are
//     screen pixels and are NOT affected by this pref; only page content scales,
//     so a higher factor fits less content into the same tile.
func WithScaleFactor(f float64) Option {
	return func(c *config) {
		if f <= 0 {
			return
		}
		c.prefs[prefDevPixelsPerPx] = strconv.FormatFloat(f, 'g', -1, 64)
	}
}

func WithCursorProfile(p CursorProfile) Option {
	return func(c *config) { c.cursorProfile = p }
}

// WithCursorTuning caps ProfileHuman's step count and per-point delay range
// (input.CursorTuning). Zero-value (default) keeps current behavior (80-step
// cap, 8-18ms delay). Use to trade human-likeness for lower per-move latency
// under heavy concurrency — see README "Cursor profile / load" guidance.
// No effect on ProfileFast / ProfileMachine.
func WithCursorTuning(t input.CursorTuning) Option {
	return func(c *config) { c.cursorTuning = t }
}

// WithFlag adds a raw Firefox CLI flag. Key is the flag name (with leading
// dash); value is the value or "" for boolean flags. Use sparingly — most
// configuration belongs in WithPref.
func WithFlag(key, value string) Option {
	return func(c *config) { c.flags[key] = value }
}

// WithPref sets a Firefox profile preference written to user.js before launch.
// E.g. WithPref("browser.startup.homepage", "about:blank").
func WithPref(key string, value any) Option {
	return func(c *config) { c.prefs[key] = value }
}

// WithModifyCmd hooks the underlying *exec.Cmd before Firefox starts.
func WithModifyCmd(fn func(*exec.Cmd)) Option {
	return func(c *config) {
		if fn != nil {
			c.modifyCmd = fn
		}
	}
}

// WithCommandTimeout overrides the default 60s backstop applied to BiDi
// commands whose context has no deadline. Pass 0 to disable the backstop
// (calls may then block until the browser closes). Negative values are
// ignored.
func WithCommandTimeout(d time.Duration) Option {
	return func(c *config) {
		if d >= 0 {
			c.commandTimeout = d
		}
	}
}

// WithWriteTimeout overrides the default 10s budget for writing a single
// WebSocket frame to the browser.
//
// The budget covers the whole write, including the wait for the shared write
// mutex — a big frame already in flight eats into the next writer's budget.
// When it expires the underlying library tears the connection down, not just
// that write, so every in-flight command on the session fails. Raising it
// (~30s) is the safer choice for fleets that push large payloads such as
// screenshots or file uploads. Pass 0 to restore the default; negative values
// are ignored.
func WithWriteTimeout(d time.Duration) Option {
	return func(c *config) {
		if d >= 0 {
			c.writeTimeout = d
		}
	}
}

// WithLaunchTimeout overrides the default 30s budget for Firefox spawn +
// BiDi WS discovery + session.new.
func WithLaunchTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.launchTimeout = d
		}
	}
}
