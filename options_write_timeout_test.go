package firefoxkit

import (
	"testing"
	"time"
)

func TestWithWriteTimeout_DefaultUnset(t *testing.T) {
	if got := defaultConfig().writeTimeout; got != 0 {
		t.Errorf("default writeTimeout: got %v, want 0 (transport default)", got)
	}
}

func TestWithWriteTimeout_SetsValue(t *testing.T) {
	cfg := defaultConfig()
	WithWriteTimeout(30 * time.Second)(cfg)
	if cfg.writeTimeout != 30*time.Second {
		t.Errorf("writeTimeout: got %v, want 30s", cfg.writeTimeout)
	}
}

func TestWithWriteTimeout_ZeroRestoresDefault(t *testing.T) {
	cfg := defaultConfig()
	WithWriteTimeout(30 * time.Second)(cfg)
	WithWriteTimeout(0)(cfg)
	if cfg.writeTimeout != 0 {
		t.Errorf("writeTimeout after 0: got %v, want 0", cfg.writeTimeout)
	}
}

func TestWithWriteTimeout_NegativeIgnored(t *testing.T) {
	cfg := defaultConfig()
	WithWriteTimeout(30 * time.Second)(cfg)
	WithWriteTimeout(-time.Second)(cfg)
	if cfg.writeTimeout != 30*time.Second {
		t.Errorf("negative ignored: got %v, want 30s", cfg.writeTimeout)
	}
}
