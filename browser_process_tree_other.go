//go:build !windows

package firefoxkit

// processTreeReaper is a no-op outside Windows: production targets Windows and
// the POSIX path keeps the previous behavior of killing the parent only.
// Kept as a real type so the Launch/Close wiring stays platform-agnostic.
type processTreeReaper struct{}

func newProcessTreeReaper(int) *processTreeReaper { return &processTreeReaper{} }

func (r *processTreeReaper) reap() error { return nil }

func (r *processTreeReaper) mode() string { return "parent kill only" }
