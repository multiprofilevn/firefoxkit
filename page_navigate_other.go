//go:build !windows

package firefoxkit

import "time"

// navigateNative is Windows-only — there is no native keyboard primitive to
// drive Firefox's address bar. Returns ErrNativeNotSupported instead of
// silently falling back to the remote path: a caller who thinks native input
// is active but isn't is a worse trap than a loud error.
func (p *Page) navigateNative(url string, timeout time.Duration) error {
	return ErrNativeNotSupported
}
