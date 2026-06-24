package firefoxkit

// Permission is a W3C-style permission name (chromekit API parity).
// Firefox does not support runtime per-origin grants; use WithPermissionsDefault
// at launch time instead. Constants here match chromekit exactly so callers can
// switch between the two kits without changing permission names.
type Permission string

const (
	PermissionGeolocation       Permission = "geolocation"
	PermissionCamera            Permission = "camera"
	PermissionMicrophone        Permission = "microphone"
	PermissionNotifications     Permission = "notifications"
	PermissionClipboardRead     Permission = "clipboard-read"
	PermissionClipboardWrite    Permission = "clipboard-write"
	PermissionMIDI              Permission = "midi"
	PermissionMIDISysex         Permission = "midi-sysex"
	PermissionBackgroundSync    Permission = "background-sync"
	PermissionPersistentStorage Permission = "persistent-storage"
)
