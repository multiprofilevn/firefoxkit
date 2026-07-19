package firefoxkit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Connect attaches to a Firefox already running with `--remote-debugging-port=N`.
// Close() disconnects but leaves Firefox running (b.closeProc=false).
//
// Options like WithUserDataDir are ignored on this path (profile is locked
// by the running process); a warning is logged when supplied.
func Connect(port int, opts ...Option) (*Browser, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if cfg.userDataDir != "" && cfg.logger != nil {
		cfg.logger.Warnf("firefoxkit.Connect: WithUserDataDir ignored — profile is locked by the running Firefox")
	}

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/session", port)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.launchTimeout)
	defer cancel()

	client, err := bidi.Dial(ctx, wsURL, bidi.Options{
		HTTPClient: http.DefaultClient,
		Logger:     toBiDiLogger(cfg.logger),
	})
	if err != nil {
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
		return nil, fmt.Errorf("%w: session.new: %v", ErrBiDiSessionFailed, err)
	}

	b := &Browser{
		client:    client,
		cfg:       cfg,
		log:       cfg.logger,
		closeProc: false,
		wsURL:     wsURL,
		sessionID: sess.SessionID,
	}
	b.initBaseCtx()

	if err := b.populateInitialPage(ctx); err != nil {
		_ = b.Close()
		return nil, err
	}
	if err := b.watchContextLifecycle(ctx); err != nil && cfg.logger != nil {
		// Degraded but usable: self-closed tabs won't be pruned automatically.
		cfg.logger.Warnf("firefoxkit: contextDestroyed subscribe: %v", err)
	}
	return b, nil
}
