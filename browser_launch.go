package firefoxkit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Launch spawns Firefox with the given options and bootstraps a BiDi session.
// Caller MUST defer b.Close() to avoid leaking the process.
//
// firefoxPath is the absolute path to firefox.exe / firefox binary. Pass ""
// to fall back to exec.LookPath("firefox") — typically fails on Windows.
func Launch(firefoxPath string, opts ...Option) (*Browser, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	if firefoxPath == "" {
		p, err := exec.LookPath("firefox")
		if err != nil {
			return nil, fmt.Errorf("%w: firefox binary not found: %v", ErrInvalidConfig, err)
		}
		firefoxPath = p
	}

	port, err := pickFreePort()
	if err != nil {
		return nil, fmt.Errorf("%w: pick port: %v", ErrInvalidConfig, err)
	}

	profileDir, cleanupProfile, err := ensureProfileDir(cfg)
	if err != nil {
		return nil, err
	}
	if err := writeUserPrefs(profileDir, cfg, port); err != nil {
		cleanupProfile()
		return nil, err
	}

	args := buildFirefoxArgs(profileDir, port, cfg)

	cmd := exec.Command(firefoxPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cleanupProfile()
		return nil, fmt.Errorf("%w: stderr pipe: %v", ErrInvalidConfig, err)
	}
	// Discard stdout so the pipe buffer never fills.
	cmd.Stdout = io.Discard

	if cfg.modifyCmd != nil {
		cfg.modifyCmd(cmd)
	}

	if err := cmd.Start(); err != nil {
		cleanupProfile()
		return nil, fmt.Errorf("%w: firefox spawn: %v", ErrInvalidConfig, err)
	}

	// Bind the process to a kill-the-whole-tree mechanism immediately: Firefox
	// starts forking content processes within milliseconds, and any of them
	// spawned before this point would survive a kill aimed at the parent.
	procTree := newProcessTreeReaper(cmd.Process.Pid)
	if cfg.logger != nil {
		cfg.logger.Debugf("firefoxkit: pid=%d process-tree reap via %s", cmd.Process.Pid, procTree.mode())
	}

	// On any failure past this point we kill the process AND clean profile.
	cleanup := func() {
		_ = procTree.reap()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		cleanupProfile()
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.launchTimeout)
	defer cancel()

	wsURL, err := discoverBiDiURL(ctx, port, stderr, cfg.logger)
	if err != nil {
		cleanup()
		return nil, err
	}

	client, err := bidi.Dial(ctx, wsURL, bidi.Options{
		HTTPClient:         http.DefaultClient,
		Logger:             toBiDiLogger(cfg.logger),
		DefaultCallTimeout: cfg.commandTimeout,
		WriteTimeout:       cfg.writeTimeout,
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: dial %s: %v", ErrBiDiSessionFailed, wsURL, err)
	}

	sess, err := client.NewSession(ctx, bidi.NewSessionParams{
		Capabilities: map[string]any{
			"alwaysMatch": map[string]any{
				"browserName":         "firefox",
				"webSocketUrl":        true,
				"acceptInsecureCerts": true,
			},
		},
	})
	if err != nil {
		_ = client.Close()
		cleanup()
		return nil, fmt.Errorf("%w: session.new: %v", ErrBiDiSessionFailed, err)
	}

	b := &Browser{
		client:         client,
		cfg:            cfg,
		log:            cfg.logger,
		proc:           cmd.Process,
		closeProc:      true,
		wsURL:          wsURL,
		sessionID:      sess.SessionID,
		cleanupProfile: cleanupProfile,
		procTree:       procTree,
		procDone:       make(chan struct{}),
	}
	b.initBaseCtx()

	// Sole owner of cmd.Wait: reaps the child when Firefox exits on its own
	// (crash, user quit) so the process entry doesn't linger until Close, and
	// closes procDone — the only trustworthy "the process is gone" signal
	// Close has after it kills the tree.
	go func() {
		defer close(b.procDone)
		_ = cmd.Wait()
	}()

	if err := b.populateInitialPage(ctx); err != nil {
		_ = b.Close() // Close kills the process and removes the temp profile.
		return nil, err
	}
	if err := b.watchContextLifecycle(ctx); err != nil && cfg.logger != nil {
		// Degraded but usable: self-closed tabs won't be pruned automatically.
		cfg.logger.Warnf("firefoxkit: contextDestroyed subscribe: %v", err)
	}
	return b, nil
}

// pickFreePort grabs an ephemeral TCP port by binding then closing. There is
// a tiny TOCTOU window before Firefox claims it; in practice acceptable
// because Firefox immediately binds on start.
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// ensureProfileDir returns an existing user-data-dir or creates a temp one.
// The cleanup func removes the temp dir; for user-supplied dirs it's a no-op.
func ensureProfileDir(cfg *config) (string, func(), error) {
	if cfg.userDataDir != "" {
		if err := os.MkdirAll(cfg.userDataDir, 0o755); err != nil {
			return "", func() {}, fmt.Errorf("%w: profile dir: %v", ErrInvalidConfig, err)
		}
		return cfg.userDataDir, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "firefoxkit-profile-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("%w: temp profile: %v", ErrInvalidConfig, err)
	}
	return dir, func() { removeProfileDirWithRetry(dir) }, nil
}

// removeProfileDirWithRetry deletes a temp profile dir, retrying briefly:
// right after the Firefox process is killed, Windows may still hold file locks
// on sqlite/cache files for a moment.
func removeProfileDirWithRetry(dir string) {
	for i := 0; i < 5; i++ {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = os.RemoveAll(dir) // last attempt; leftover dir is logged nowhere — best-effort
}

// writeUserPrefs assembles user.js with the prefs needed to enable BiDi +
// caller-supplied prefs + proxy if set. Firefox reads user.js on startup
// and copies entries into prefs.js, so options like the remote agent port
// must be set there OR via CLI flags.
func writeUserPrefs(profileDir string, cfg *config, port int) error {
	prefs := map[string]any{
		// BiDi / Remote Agent.
		"remote.active-protocols":       3, // WebDriver classic | BiDi
		"remote.log.level":              "Info",
		"devtools.chrome.enabled":       true,
		"browser.shell.checkDefaultBrowser": false,
		"browser.startup.homepage_override.mstone": "ignore",
		"browser.startup.homepage":      "about:blank",
		"toolkit.telemetry.reportingpolicy.firstRun": false,
		"datareporting.policy.firstRunURL": "",
		// Avoid first-run / what's-new pages that steal focus.
		"startup.homepage_welcome_url":  "about:blank",
		"startup.homepage_welcome_url.additional": "",
	}
	if cfg.proxy != "" {
		host, portStr, err := splitHostPort(cfg.proxy)
		if err != nil {
			return fmt.Errorf("%w: proxy: %v", ErrInvalidConfig, err)
		}
		pn, _ := strconv.Atoi(portStr)
		prefs["network.proxy.type"] = 1
		prefs["network.proxy.http"] = host
		prefs["network.proxy.http_port"] = pn
		prefs["network.proxy.ssl"] = host
		prefs["network.proxy.ssl_port"] = pn
		prefs["network.proxy.share_proxy_settings"] = true
	}
	// User-supplied prefs win on conflict.
	for k, v := range cfg.prefs {
		prefs[k] = v
	}

	var sb strings.Builder
	keys := make([]string, 0, len(prefs))
	for k := range prefs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := json.Marshal(prefs[k])
		sb.WriteString(fmt.Sprintf("user_pref(%q, %s);\n", k, string(v)))
	}
	_ = port // port is set via CLI flag, not pref — kept for future use

	path := filepath.Join(profileDir, "user.js")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("%w: write user.js: %v", ErrInvalidConfig, err)
	}
	return nil
}

func splitHostPort(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}
	return host, port, nil
}

// buildFirefoxArgs returns the firefox CLI args for BiDi launch.
func buildFirefoxArgs(profileDir string, port int, cfg *config) []string {
	args := []string{
		"--remote-debugging-port", strconv.Itoa(port),
		"-profile", profileDir,
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

// bidiListenRegex matches the stderr line Firefox prints when the BiDi
// endpoint opens. Format: "WebDriver BiDi listening on ws://127.0.0.1:NNNN"
var bidiListenRegex = regexp.MustCompile(`WebDriver BiDi listening on (ws://[^\s]+)`)

// bidiSessionURL normalizes the base URL Firefox prints ("ws://host:port",
// no path) to the actual BiDi WebSocket endpoint ("ws://host:port/session").
// Dialing the bare base returns HTTP 200 (not a WS upgrade); the "/session"
// path is what performs the WebSocket handshake. Idempotent — leaves an URL
// that already ends in "/session" untouched.
func bidiSessionURL(raw string) string {
	u := strings.TrimRight(raw, "/")
	if strings.HasSuffix(u, "/session") {
		return u
	}
	return u + "/session"
}

// discoverBiDiURL races two strategies: stderr regex scan + HTTP probe at
// http://127.0.0.1:port/session/new. Returns the WS URL of the first
// strategy to succeed.
func discoverBiDiURL(ctx context.Context, port int, stderr io.ReadCloser, logger Logger) (string, error) {
	type result struct {
		url string
		err error
	}
	results := make(chan result, 2)
	var stderrBuf strings.Builder
	var mu sync.Mutex

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			// Cap the retained buffer so a Firefox that never prints the
			// expected line (log format change) can't grow it unbounded.
			if stderrBuf.Len() < 64*1024 {
				stderrBuf.WriteString(line)
				stderrBuf.WriteString("\n")
			}
			mu.Unlock()
			if m := bidiListenRegex.FindStringSubmatch(line); len(m) == 2 {
				select {
				case results <- result{url: bidiSessionURL(m[1])}:
				case <-ctx.Done():
				}
				// Keep draining so the stderr pipe never blocks Firefox.
				for scanner.Scan() {
				}
				return
			}
		}
	}()

	go func() {
		// HTTP probe — Firefox does NOT expose a /json/version endpoint like
		// Chrome. We just poll the BiDi endpoint URL via a TCP dial; once
		// the port is listening, BiDi is ready.
		base := fmt.Sprintf("ws://127.0.0.1:%d/session", port)
		probeAddr := fmt.Sprintf("127.0.0.1:%d", port)
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			conn, err := net.DialTimeout("tcp", probeAddr, 200*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				select {
				case results <- result{url: base}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()

	select {
	case r := <-results:
		if r.err != nil {
			return "", r.err
		}
		if logger != nil {
			logger.Debugf("firefoxkit: BiDi endpoint at %s", r.url)
		}
		return r.url, nil
	case <-ctx.Done():
		mu.Lock()
		tail := stderrBuf.String()
		mu.Unlock()
		return "", fmt.Errorf("%w: stderr tail: %s", ErrFirefoxNotReady, truncate(tail, 800))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
