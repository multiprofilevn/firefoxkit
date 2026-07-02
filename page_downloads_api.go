package firefoxkit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// downloadsAPI is the per-Page download state — a single subscribed Wait
// channel + the optional configured directory.
type downloadsAPI struct {
	mu      sync.Mutex
	dir     string
	pending chan downloadEndPayload
	sub     *bidi.Subscription
}

type downloadEndPayload struct {
	Path   string
	URL    string
	Status string
}

// Downloads returns the per-Page download namespace.
func (p *Page) Downloads() *DownloadsAPI {
	p.dlOnce.Do(func() {
		p.dl = &downloadsAPI{pending: make(chan downloadEndPayload, 8)}
	})
	return &DownloadsAPI{p: p}
}

// DownloadsAPI is the public façade for the download namespace.
type DownloadsAPI struct{ p *Page }

// SetPath records the directory Firefox should save downloads into. Only
// effective when set BEFORE Launch (written into user.js as
// `browser.download.dir`). Setting after Launch returns nil but takes no
// effect (Firefox reads prefs at startup).
func (d *DownloadsAPI) SetPath(dir string) error {
	d.p.dl.mu.Lock()
	d.p.dl.dir = dir
	d.p.dl.mu.Unlock()
	return nil
}

// Wait blocks until the next download completes, or timeout. Returns the
// local file path of the completed download.
func (d *DownloadsAPI) Wait(timeout time.Duration) (string, error) {
	if err := d.ensureSubscribed(d.p.ctx()); err != nil {
		return "", err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	tctx, cancel := d.p.opCtx(timeout)
	defer cancel()
	select {
	case ev := <-d.p.dl.pending:
		if ev.Status != "complete" {
			return "", ErrDownloadCanceled
		}
		return ev.Path, nil
	case <-tctx.Done():
		if tctx.Err() == context.DeadlineExceeded {
			return "", ErrTimeout
		}
		return "", ErrContextCanceled
	}
}

func (d *DownloadsAPI) ensureSubscribed(ctx context.Context) error {
	d.p.dl.mu.Lock()
	defer d.p.dl.mu.Unlock()
	if d.p.dl.sub != nil {
		return nil
	}
	if err := d.p.browser.client.SubscribeEvents(ctx, bidi.SubscribeParams{
		Events:   []string{"browsingContext.downloadEnd"},
		Contexts: []string{d.p.contextID},
	}); err != nil {
		return err
	}
	d.p.dl.sub = d.p.browser.client.Subscribe("browsingContext.downloadEnd", func(params json.RawMessage) {
		var ev bidi.DownloadEndEvent
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}
		if ev.Context != d.p.contextID {
			return
		}
		select {
		case d.p.dl.pending <- downloadEndPayload{Path: ev.FilePath, URL: ev.URL, Status: ev.Status}:
		default:
			// Buffer full — drop oldest, push new. Keeps Wait responsive on bursts.
			select {
			case <-d.p.dl.pending:
			default:
			}
			d.p.dl.pending <- downloadEndPayload{Path: ev.FilePath, URL: ev.URL, Status: ev.Status}
		}
	})
	return nil
}
