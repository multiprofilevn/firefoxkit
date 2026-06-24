package firefoxkit

import (
	"fmt"
	"os"
	"os/exec"
)

// SpawnedFirefox is a spawn-only handle to a Firefox process started WITHOUT
// remote debugging or automation prefs. Use it for anti-detect profiles (e.g.
// GPM Firefox) whose profile dir already carries a complete prefs.js: Launch's
// user.js (remote.active-protocols, devtools.chrome.enabled, ...) and
// --remote-debugging-port would pollute that fingerprint and be trivially
// detectable. There is no BiDi session — control is out of scope here; readiness
// is process-alive (see Done).
type SpawnedFirefox struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// Spawn starts Firefox against an existing profile dir and returns as soon as
// the process is running. Unlike Launch it does NOT write user.js, does NOT pass
// --remote-debugging-port, and opens no BiDi session — nothing touches the
// profile's prefs beyond what the caller already wrote. Proxy is intentionally
// not injected: anti-detect profiles carry network.proxy.* inside their prefs.js.
func Spawn(firefoxPath string, opts ...Option) (*SpawnedFirefox, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.userDataDir == "" {
		return nil, fmt.Errorf("%w: Spawn requires WithUserDataDir (the prepared profile dir)", ErrInvalidConfig)
	}
	if firefoxPath == "" {
		p, err := exec.LookPath("firefox")
		if err != nil {
			return nil, fmt.Errorf("%w: firefox binary not found: %v", ErrInvalidConfig, err)
		}
		firefoxPath = p
	}
	if err := os.MkdirAll(cfg.userDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: profile dir: %v", ErrInvalidConfig, err)
	}

	cmd := exec.Command(firefoxPath, buildSpawnArgs(cfg)...)
	if cfg.modifyCmd != nil {
		cfg.modifyCmd(cmd)
	}
	if err := cmd.Start(); err != nil {
		// Not ErrInvalidConfig: config validated and binary resolved — this is a
		// runtime spawn failure (e.g. binary missing/locked).
		return nil, fmt.Errorf("firefox spawn: %w", err)
	}

	s := &SpawnedFirefox{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()
	return s, nil
}

// buildSpawnArgs returns spawn-only CLI args: profile + instance isolation, NO
// remote-debugging port. Extra cfg.flags are appended for callers that need GPM
// FirefoxCore-specific switches.
func buildSpawnArgs(cfg *config) []string {
	args := []string{
		"-profile", cfg.userDataDir,
		"-no-remote",
		"-new-instance",
	}
	if cfg.headless {
		args = append(args, "-headless")
	}
	for k, v := range cfg.flags {
		if v == "" {
			args = append(args, k)
		} else {
			args = append(args, k, v)
		}
	}
	return args
}

// PID returns the Firefox process id, or 0 if the process never started.
func (s *SpawnedFirefox) PID() int {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Done is closed when the Firefox process exits. This is the spawn-only
// readiness/heartbeat signal (process-alive), since there is no BiDi session.
func (s *SpawnedFirefox) Done() <-chan struct{} { return s.done }

// Kill terminates the Firefox process. Safe to call multiple times.
func (s *SpawnedFirefox) Kill() error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}
