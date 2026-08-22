package firefoxkit

import (
	"fmt"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// InstallWebExtension installs an unpacked extension directory (or .xpi) as a
// temporary addon via BiDi webExtension.install. Returns the extension id.
// Temporary install bypasses signature enforcement. Requires Firefox 132+.
func (b *Browser) InstallWebExtension(path string) (string, error) {
	if b == nil || b.client == nil {
		return "", fmt.Errorf("firefoxkit: browser not connected")
	}
	res, err := b.client.InstallWebExtension(b.ctx(), bidi.InstallWebExtensionParams{
		ExtensionData: bidi.ExtensionData{Type: "path", Path: path},
	})
	if err != nil {
		return "", fmt.Errorf("webExtension.install: %w", err)
	}
	return res.Extension, nil
}
