package firefoxkit

import (
	"os/exec"
	"time"
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
	mousePathURL  string
	flags         map[string]string
	prefs         map[string]any
	modifyCmd      func(*exec.Cmd)
	launchTimeout  time.Duration
	commandTimeout time.Duration
}

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

// WithMousePathURL sets the full URL of a mouse-positions endpoint that returns
// a high-entropy {x,y,delay} cursor path. When set, ProfileHuman moves fetch
// from it and replay; empty (default) keeps the pure local bezier engine.
func WithMousePathURL(url string) Option {
	return func(c *config) { c.mousePathURL = url }
}

func WithNativeWindow(x, y int, scale float64) Option {
	return func(c *config) {
		if scale <= 0 {
			scale = 1.0
		}
		c.nativeWindow = nativeWindow{X: x, Y: y, Scale: scale}
	}
}

func WithCursorProfile(p CursorProfile) Option {
	return func(c *config) { c.cursorProfile = p }
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

// WithLaunchTimeout overrides the default 30s budget for Firefox spawn +
// BiDi WS discovery + session.new.
func WithLaunchTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.launchTimeout = d
		}
	}
}
