//go:build !windows

package firefoxkit

import (
	"errors"
	"testing"
	"time"
)

// 6. TestNavigate_NativeNotSupportedOnOther: on a non-Windows build, native
// Navigate must return ErrNativeNotSupported — never silently fall back to
// the remote path. Only compiled/run on non-Windows (GOOS=linux go build
// verifies this file compiles; it does not run on the Windows dev host).
func TestNavigate_NativeNotSupportedOnOther(t *testing.T) {
	b := newTestBrowser()
	b.cfg = defaultConfig()
	b.cfg.inputBackend = BackendNative
	p := &Page{browser: b}

	err := p.Navigate("https://example.com", time.Second)
	if !errors.Is(err, ErrNativeNotSupported) {
		t.Fatalf("Navigate error = %v, want ErrNativeNotSupported", err)
	}
}
