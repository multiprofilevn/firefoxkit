package firefoxkit

import (
	"strings"
	"testing"
)

// TestWithScaleFactor_WritesDevPixelsPref: the pref is a CHAR pref in Firefox —
// it must land in user.js as a quoted string, not a bare number.
func TestWithScaleFactor_WritesDevPixelsPref(t *testing.T) {
	cfg := defaultConfig()
	WithScaleFactor(1.5)(cfg)
	if got := cfg.prefs[prefDevPixelsPerPx]; got != "1.5" {
		t.Fatalf("scale pref: got %#v, want \"1.5\"", got)
	}
	dir := t.TempDir()
	if err := writeUserPrefs(dir, cfg, 0); err != nil {
		t.Fatalf("writeUserPrefs: %v", err)
	}
	data := readFile(t, dir+"/user.js")
	if !strings.Contains(data, `"layout.css.devPixelsPerPx", "1.5"`) {
		t.Fatalf("scale pref missing from user.js:\n%s", data)
	}
}

// TestWithScaleFactor_IntegralValue: 2 must serialize as "2", never "2.000000".
func TestWithScaleFactor_IntegralValue(t *testing.T) {
	cfg := defaultConfig()
	WithScaleFactor(2)(cfg)
	if got := cfg.prefs[prefDevPixelsPerPx]; got != "2" {
		t.Fatalf("scale pref: got %#v, want \"2\"", got)
	}
}

// TestWithScaleFactor_NonPositiveIgnored: <= 0 means "leave Firefox on system
// DPI" — the pref must not be written at all (Firefox's own default is -1.0).
func TestWithScaleFactor_NonPositiveIgnored(t *testing.T) {
	for _, f := range []float64{0, -1, -2.5} {
		cfg := defaultConfig()
		WithScaleFactor(f)(cfg)
		if _, ok := cfg.prefs[prefDevPixelsPerPx]; ok {
			t.Fatalf("scale %g: pref must be absent, got %#v", f, cfg.prefs[prefDevPixelsPerPx])
		}
	}
}

func TestWithPermissionsDefault_Allow(t *testing.T) {
	cfg := defaultConfig()
	WithPermissionsDefault(PermGeo, true)(cfg)
	if got := cfg.prefs["permissions.default.geo"]; got != 1 {
		t.Errorf("geo allow: got %v, want 1", got)
	}
}

func TestWithPermissionsDefault_Block(t *testing.T) {
	cfg := defaultConfig()
	WithPermissionsDefault(PermCamera, false)(cfg)
	if got := cfg.prefs["permissions.default.camera"]; got != 2 {
		t.Errorf("camera block: got %v, want 2", got)
	}
}

func TestWithAcceptDownloads(t *testing.T) {
	cfg := defaultConfig()
	WithAcceptDownloads(true)(cfg)
	if cfg.prefs["browser.download.folderList"] != 2 {
		t.Errorf("folderList not set")
	}
	if cfg.prefs["browser.download.useDownloadDir"] != true {
		t.Errorf("useDownloadDir not set")
	}
}

func TestWithDownloadDir(t *testing.T) {
	cfg := defaultConfig()
	WithDownloadDir(`C:\tmp\dl`)(cfg)
	if cfg.prefs["browser.download.dir"] != `C:\tmp\dl` {
		t.Errorf("dir not set: %v", cfg.prefs["browser.download.dir"])
	}
}

func TestGrantPermissions_TypedSig_NoopRuntime(t *testing.T) {
	b := &Browser{}
	// GrantPermissions now accepts ...Permission (typed, not string).
	// Firefox runtime grants are a no-op — must return nil, not error.
	if err := b.GrantPermissions("https://example.com", PermissionGeolocation); err != nil {
		t.Errorf("GrantPermissions no-op: got %v, want nil", err)
	}
	// Multiple permissions must also be fine.
	if err := b.GrantPermissions("https://example.com", PermissionCamera, PermissionMicrophone); err != nil {
		t.Errorf("GrantPermissions multi: got %v, want nil", err)
	}
	// Empty perm list.
	if err := b.GrantPermissions("https://example.com"); err != nil {
		t.Errorf("GrantPermissions empty: got %v, want nil", err)
	}
}

func TestResetPermissions_ReturnsUnsupported(t *testing.T) {
	b := &Browser{}
	if err := b.ResetPermissions(); err != ErrPermissionsRuntimeUnsupported {
		t.Errorf("got %v, want ErrPermissionsRuntimeUnsupported", err)
	}
}
