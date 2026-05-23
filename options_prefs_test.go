package firefoxkit

import (
	"errors"
	"testing"
)

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

func TestGrantPermissions_ReturnsUnsupported(t *testing.T) {
	b := &Browser{}
	err := b.GrantPermissions("https://example.com", "geo")
	if !errors.Is(err, ErrPermissionsRuntimeUnsupported) {
		t.Errorf("got %v, want ErrPermissionsRuntimeUnsupported", err)
	}
}

func TestResetPermissions_ReturnsUnsupported(t *testing.T) {
	b := &Browser{}
	if err := b.ResetPermissions(); !errors.Is(err, ErrPermissionsRuntimeUnsupported) {
		t.Errorf("got %v, want ErrPermissionsRuntimeUnsupported", err)
	}
}
