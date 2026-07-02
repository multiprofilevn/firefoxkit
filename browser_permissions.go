package firefoxkit

// GrantPermissions matches the chromekit signature for API parity but is a
// deliberate runtime no-op in firefoxkit.
//
// Firefox's WebDriver BiDi protocol does not expose a per-origin permission
// grant API, and manipulating permissions.sqlite while Firefox is running is
// unsafe. As a result this method always returns nil (success) while emitting
// a warning so callers are aware no grant was applied.
//
// Use launch-time options instead:
//
//	WithPermissionsDefault(PermGeo, true)       — blanket-allow geolocation
//	WithPref("permissions.default.<name>", 1)   — raw escape hatch
//
// See also: ErrPermissionsRuntimeUnsupported (returned by older versions),
// type Permission and its constants for the supported permission names.
func (b *Browser) GrantPermissions(origin string, perms ...Permission) error {
	if b.log != nil {
		b.log.Warnf(
			"firefoxkit: GrantPermissions is a no-op — Firefox does not support "+
				"runtime per-origin permission grants via BiDi (origin=%s perms=%v). "+
				"Use WithPermissionsDefault at Launch instead.",
			origin, perms,
		)
	}
	return nil
}

// ResetPermissions is intentionally NOT supported at runtime. See
// GrantPermissions for the launch-time alternative.
func (b *Browser) ResetPermissions() error {
	return ErrPermissionsRuntimeUnsupported
}
