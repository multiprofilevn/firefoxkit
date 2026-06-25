package bidi

import "context"

// InstallWebExtensionParams configures webExtension.install.
type InstallWebExtensionParams struct {
	ExtensionData ExtensionData `json:"extensionData"`
}

// ExtensionData identifies the extension to install. Type "path" loads an
// unpacked extension directory (or .xpi) from disk as a temporary addon —
// no signing required (same as about:debugging temporary load).
type ExtensionData struct {
	Type  string `json:"type"`            // "path" | "archivePath" | "base64"
	Path  string `json:"path,omitempty"`
	Value string `json:"value,omitempty"`
}

// InstallWebExtensionResult is the response of webExtension.install.
type InstallWebExtensionResult struct {
	Extension string `json:"extension"` // installed extension id
}

// InstallWebExtension installs a web extension via BiDi webExtension.install.
// For unpacked directories use ExtensionData{Type:"path", Path:"<dir>"}.
// Temporary installs bypass Firefox signature enforcement (requires FF 132+).
func (c *Client) InstallWebExtension(ctx context.Context, p InstallWebExtensionParams) (*InstallWebExtensionResult, error) {
	var out InstallWebExtensionResult
	if err := c.Send(ctx, "webExtension.install", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
