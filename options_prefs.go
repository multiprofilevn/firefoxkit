package firefoxkit

// Permission names that map to Firefox `permissions.default.<name>` prefs.
// Firefox values: 0 = ask (default), 1 = allow, 2 = block.
const (
	PermGeo           = "geo"
	PermNotifications = "desktop-notification"
	PermCamera        = "camera"
	PermMicrophone    = "microphone"
	PermXR            = "xr"
)

// WithPermissionsDefault sets a blanket default for the named permission.
// allow=true → "always allow"; allow=false → "always block". Per-origin
// granular control isn't possible without manipulating permissions.sqlite —
// see ErrPermissionsRuntimeUnsupported.
func WithPermissionsDefault(perm string, allow bool) Option {
	return func(c *config) {
		value := 2 // block
		if allow {
			value = 1
		}
		c.prefs["permissions.default."+perm] = value
	}
}

// WithAcceptDownloads disables the download confirmation prompt so files
// land in the configured download directory without user interaction. Pairs
// with WithDownloadDir.
func WithAcceptDownloads(accept bool) Option {
	return func(c *config) {
		if accept {
			c.prefs["browser.download.useDownloadDir"] = true
			c.prefs["browser.download.folderList"] = 2
			c.prefs["browser.download.manager.showWhenStarting"] = false
			c.prefs["browser.download.alwaysOpenPanel"] = false
		}
	}
}

// WithDownloadDir sets the directory Firefox saves downloads into. Combined
// with WithAcceptDownloads to skip the confirmation dialog.
func WithDownloadDir(dir string) Option {
	return func(c *config) {
		c.prefs["browser.download.dir"] = dir
		c.prefs["browser.download.folderList"] = 2
	}
}
