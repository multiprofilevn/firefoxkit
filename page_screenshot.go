package firefoxkit

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"image/png"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// Screenshot captures the current viewport as PNG bytes.
func (p *Page) Screenshot() ([]byte, error) {
	return p.capture(bidi.CaptureScreenshotParams{
		Context: p.contextID,
		Origin:  "viewport",
	})
}

// FullScreenshot captures the full document. quality in [1, 100] re-encodes
// as JPEG; quality >= 100 keeps PNG.
func (p *Page) FullScreenshot(quality int) ([]byte, error) {
	raw, err := p.capture(bidi.CaptureScreenshotParams{
		Context: p.contextID,
		Origin:  "document",
	})
	if err != nil {
		return nil, err
	}
	if quality >= 100 || quality <= 0 {
		return raw, nil
	}
	return reencodeAsJPEG(raw, quality)
}

// ScreenshotClip captures a sub-rectangle of the viewport.
func (p *Page) ScreenshotClip(x, y, w, h float64) ([]byte, error) {
	return p.capture(bidi.CaptureScreenshotParams{
		Context: p.contextID,
		Origin:  "viewport",
		Clip: &bidi.ScreenshotClip{
			Type: "box", X: x, Y: y, Width: w, Height: h,
		},
	})
}

// ScreenshotElement captures the bounding box of the first element matching sel.
// Scrolls into view first.
func (p *Page) ScreenshotElement(sel string) ([]byte, error) {
	if err := p.ScrollIntoView(sel); err != nil {
		return nil, err
	}
	bb, err := p.BoundingBox(sel)
	if err != nil {
		return nil, err
	}
	if bb.Width == 0 || bb.Height == 0 {
		return nil, fmt.Errorf("%w: element has zero size", ErrPageNotFound)
	}
	return p.ScreenshotClip(bb.X, bb.Y, bb.Width, bb.Height)
}

func (p *Page) capture(params bidi.CaptureScreenshotParams) ([]byte, error) {
	res, err := p.browser.client.CaptureScreenshot(p.ctx(), params)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return nil, fmt.Errorf("firefoxkit: decode screenshot: %w", err)
	}
	return data, nil
}

// reencodeAsJPEG decodes PNG → re-encodes JPEG at the given quality.
// BiDi captureScreenshot's `format` parameter advertises JPEG but Firefox
// support varied across early FF 129-132 builds; client-side re-encode is
// the safe path until we pin a known-good version.
func reencodeAsJPEG(pngData []byte, quality int) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("firefoxkit: decode PNG for JPEG re-encode: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("firefoxkit: encode JPEG: %w", err)
	}
	return buf.Bytes(), nil
}
