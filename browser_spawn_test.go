package firefoxkit

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildSpawnArgs_NoRemoteDebugging(t *testing.T) {
	cfg := defaultConfig()
	cfg.userDataDir = `C:\profiles\p1`
	args := buildSpawnArgs(cfg)

	joined := strings.Join(args, " ")
	// The whole point of Spawn vs Launch: no automation surface.
	if strings.Contains(joined, "--remote-debugging-port") {
		t.Fatalf("spawn args must not expose --remote-debugging-port: %q", joined)
	}
	for _, want := range []string{"-profile", `C:\profiles\p1`, "-no-remote", "-new-instance"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("spawn args missing %q: %q", want, joined)
		}
	}
}

func TestBuildSpawnArgs_HeadlessAndFlags(t *testing.T) {
	cfg := defaultConfig()
	cfg.userDataDir = "/tmp/p"
	cfg.headless = true
	cfg.flags = map[string]string{"-foreground": ""}
	args := buildSpawnArgs(cfg)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-headless") {
		t.Fatalf("expected -headless: %q", joined)
	}
	if !strings.Contains(joined, "-foreground") {
		t.Fatalf("expected appended flag -foreground: %q", joined)
	}
}

func TestSpawn_RequiresUserDataDir(t *testing.T) {
	_, err := Spawn(`C:\firefox\firefox.exe`)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig when userDataDir missing, got %v", err)
	}
}
