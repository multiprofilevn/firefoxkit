package firefoxkit

import "fmt"

// GrantPermissions is intentionally NOT supported at runtime in firefoxkit.
// Firefox's BiDi implementation does not expose a per-origin permission API,
// and permissions.sqlite manipulation while Firefox is running is unsafe.
//
// Use launch-time options instead:
//   - WithPermissionsDefault(PermGeo, true) — blanket-allow geolocation
//   - WithPref("permissions.default.<name>", 1) — raw escape hatch
//
// See ErrPermissionsRuntimeUnsupported for the canonical error.
func (b *Browser) GrantPermissions(origin string, perms ...string) error {
	return fmt.Errorf("%w: origin=%s perms=%v", ErrPermissionsRuntimeUnsupported, origin, perms)
}

// ResetPermissions is intentionally NOT supported at runtime. See
// GrantPermissions for the launch-time alternative.
func (b *Browser) ResetPermissions() error {
	return ErrPermissionsRuntimeUnsupported
}
