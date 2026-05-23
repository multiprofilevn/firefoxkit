package firefoxkit

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateConfig_NativeHeadlessRejected(t *testing.T) {
	cfg := defaultConfig()
	cfg.headless = true
	cfg.inputBackend = BackendNative
	if err := validateConfig(cfg); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestValidateConfig_OK(t *testing.T) {
	cfg := defaultConfig()
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.inputBackend != BackendBiDi {
		t.Errorf("default backend = %v, want BackendBiDi", cfg.inputBackend)
	}
	if cfg.cursorProfile != CursorHuman {
		t.Errorf("default cursor = %v, want CursorHuman", cfg.cursorProfile)
	}
	if cfg.launchTimeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", cfg.launchTimeout)
	}
}

func TestOptions_Apply(t *testing.T) {
	cfg := defaultConfig()
	WithHeadless(true)(cfg)
	WithProxy("127.0.0.1:8888")(cfg)
	WithUserDataDir(`C:\tmp\ffprofile`)(cfg)
	WithInputBackend(BackendNative)(cfg)
	WithCursorProfile(CursorFast)(cfg)
	WithFlag("-foreground", "")(cfg)
	WithPref("network.cookie.cookieBehavior", 0)(cfg)
	WithLaunchTimeout(45 * time.Second)(cfg)

	switch {
	case !cfg.headless:
		t.Error("headless not set")
	case cfg.proxy != "127.0.0.1:8888":
		t.Error("proxy not set")
	case cfg.userDataDir != `C:\tmp\ffprofile`:
		t.Error("userDataDir not set")
	case cfg.inputBackend != BackendNative:
		t.Error("inputBackend not set")
	case cfg.cursorProfile != CursorFast:
		t.Error("cursorProfile not set")
	case cfg.flags["-foreground"] != "":
		t.Error("flag not set")
	case cfg.prefs["network.cookie.cookieBehavior"] != 0:
		t.Error("pref not set")
	case cfg.launchTimeout != 45*time.Second:
		t.Error("launchTimeout not set")
	}
}

func TestWithLogger_NilIgnored(t *testing.T) {
	cfg := defaultConfig()
	WithLogger(nil)(cfg)
	if cfg.logger == nil {
		t.Fatal("logger nil after WithLogger(nil)")
	}
	if _, ok := cfg.logger.(noopLogger); !ok {
		t.Fatal("expected noopLogger preserved")
	}
}

func TestWithNativeWindow_ZeroScaleNormalized(t *testing.T) {
	cfg := defaultConfig()
	WithNativeWindow(100, 200, 0)(cfg)
	if cfg.nativeWindow.Scale != 1.0 {
		t.Fatalf("scale = %v, want 1.0", cfg.nativeWindow.Scale)
	}
	if cfg.nativeWindow.X != 100 || cfg.nativeWindow.Y != 200 {
		t.Fatalf("xy = %v,%v", cfg.nativeWindow.X, cfg.nativeWindow.Y)
	}
}

func TestBuildFirefoxArgs_Headless(t *testing.T) {
	cfg := defaultConfig()
	cfg.headless = true
	args := buildFirefoxArgs("/tmp/p", 9444, cfg)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--remote-debugging-port 9444") {
		t.Errorf("missing port flag: %s", joined)
	}
	if !strings.Contains(joined, "-profile /tmp/p") {
		t.Errorf("missing profile: %s", joined)
	}
	if !strings.Contains(joined, "-no-remote") {
		t.Errorf("missing -no-remote: %s", joined)
	}
	if !strings.Contains(joined, "-headless") {
		t.Errorf("missing -headless: %s", joined)
	}
}

func TestBuildFirefoxArgs_CustomFlag(t *testing.T) {
	cfg := defaultConfig()
	cfg.flags["-private"] = ""
	cfg.flags["-MOZ_LOG"] = "timestamp,sync"
	args := buildFirefoxArgs("/tmp/p", 9445, cfg)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-private") {
		t.Errorf("missing -private: %s", joined)
	}
	if !strings.Contains(joined, "-MOZ_LOG timestamp,sync") {
		t.Errorf("missing -MOZ_LOG with value: %s", joined)
	}
}

func TestPickFreePort(t *testing.T) {
	p, err := pickFreePort()
	if err != nil {
		t.Fatalf("pickFreePort: %v", err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("port out of range: %d", p)
	}
}

func TestSplitHostPort(t *testing.T) {
	h, p, err := splitHostPort("proxy.local:3128")
	if err != nil {
		t.Fatalf("splitHostPort: %v", err)
	}
	if h != "proxy.local" || p != "3128" {
		t.Fatalf("got %s:%s", h, p)
	}
}

func TestMatchPage(t *testing.T) {
	cases := []struct {
		have, want string
		hit        bool
	}{
		{"https://Example.com/foo", "example.com", true},
		{"https://example.com", "google", false},
		{"", "x", false},
	}
	for _, c := range cases {
		if got := matchPage(c.have, c.want); got != c.hit {
			t.Errorf("matchPage(%q, %q) = %v, want %v", c.have, c.want, got, c.hit)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abc", 10) != "abc" {
		t.Error("short string mangled")
	}
	out := truncate(strings.Repeat("x", 100), 10)
	if !strings.HasPrefix(out, "...") || len(out) != 13 {
		t.Errorf("truncate: %q", out)
	}
}

func TestBiDiListenRegex(t *testing.T) {
	line := "1718000000000\tWebDriver BiDi listening on ws://127.0.0.1:54321/session"
	m := bidiListenRegex.FindStringSubmatch(line)
	if len(m) != 2 {
		t.Fatalf("no match: %q", line)
	}
	if m[1] != "ws://127.0.0.1:54321/session" {
		t.Fatalf("captured = %q", m[1])
	}
}

func TestEnsureProfileDir_Temp(t *testing.T) {
	cfg := defaultConfig()
	dir, cleanup, err := ensureProfileDir(cfg)
	if err != nil {
		t.Fatalf("ensureProfileDir: %v", err)
	}
	defer cleanup()
	if dir == "" {
		t.Fatal("empty dir")
	}
	if !strings.Contains(dir, "firefoxkit-profile") {
		t.Errorf("unexpected dir name: %s", dir)
	}
}

func TestWriteUserPrefs_RespectsCallerOverride(t *testing.T) {
	cfg := defaultConfig()
	cfg.prefs["browser.startup.homepage"] = "https://custom.local"
	dir := t.TempDir()
	if err := writeUserPrefs(dir, cfg, 9999); err != nil {
		t.Fatalf("writeUserPrefs: %v", err)
	}
	// Caller's value must win the merge.
	data := readFile(t, dir+"/user.js")
	if !strings.Contains(data, `"browser.startup.homepage", "https://custom.local"`) {
		t.Fatalf("override lost:\n%s", data)
	}
	if !strings.Contains(data, `"remote.active-protocols", 3`) {
		t.Fatalf("default BiDi pref missing:\n%s", data)
	}
}

func TestWriteUserPrefs_ProxyExpanded(t *testing.T) {
	cfg := defaultConfig()
	cfg.proxy = "10.0.0.5:3128"
	dir := t.TempDir()
	if err := writeUserPrefs(dir, cfg, 0); err != nil {
		t.Fatalf("writeUserPrefs: %v", err)
	}
	data := readFile(t, dir+"/user.js")
	for _, want := range []string{
		`"network.proxy.type", 1`,
		`"network.proxy.http", "10.0.0.5"`,
		`"network.proxy.http_port", 3128`,
		`"network.proxy.ssl", "10.0.0.5"`,
	} {
		if !strings.Contains(data, want) {
			t.Errorf("missing %q in:\n%s", want, data)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
