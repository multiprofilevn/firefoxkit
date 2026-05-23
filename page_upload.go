package firefoxkit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetInputFiles assigns paths to the <input type="file"> matched by sel.
// Reads each file into memory and synthesizes a DataTransfer via JS so
// downstream change-event handlers see the same FileList a user pick would
// produce. Works for size-bounded files (current limit: 50 MB combined).
//
// Limitation: sites that intercept the input's click via a custom file
// picker (e.g. opening a dialog with stricter validation) may not observe
// the synthesized FileList. Use BackendNative + a real OS dialog in those
// cases — that path lands in a follow-up.
func (p *Page) SetInputFiles(ctx context.Context, sel string, paths ...string) error {
	type fileBlob struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Base64  string `json:"base64"`
	}
	blobs := make([]fileBlob, 0, len(paths))
	const maxTotal = 50 * 1024 * 1024
	total := int64(0)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("firefoxkit: SetInputFiles read %s: %w", path, err)
		}
		total += int64(len(data))
		if total > maxTotal {
			return fmt.Errorf("firefoxkit: SetInputFiles: combined size > 50MB")
		}
		blobs = append(blobs, fileBlob{
			Name:   filepath.Base(path),
			Type:   guessMime(path),
			Base64: base64.StdEncoding.EncodeToString(data),
		})
	}
	payload, _ := json.Marshal(blobs)
	expr := fmt.Sprintf(`(function(){
		const blobs = %s;
		const el = document.querySelector(%q);
		if (!el) return false;
		const dt = new DataTransfer();
		for (const b of blobs) {
			const bin = atob(b.base64);
			const arr = new Uint8Array(bin.length);
			for (let i=0;i<bin.length;i++) arr[i] = bin.charCodeAt(i);
			dt.items.add(new File([arr], b.name, {type: b.type}));
		}
		el.files = dt.files;
		el.dispatchEvent(new Event("input", {bubbles:true}));
		el.dispatchEvent(new Event("change", {bubbles:true}));
		return true;
	})()`, string(payload), sel)
	var ok bool
	if err := p.Evaluate(ctx, expr, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrPageNotFound
	}
	return nil
}

// guessMime picks a coarse MIME type from the file extension. Good enough
// for sites that sniff by extension — for stricter sites, callers should
// inject the correct MIME themselves via a follow-up `el.files` mutation.
func guessMime(path string) string {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".txt", ".log":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
