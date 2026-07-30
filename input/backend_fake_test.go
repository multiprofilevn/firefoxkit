package input

import (
	"context"
	"fmt"
	"strings"
)

// fakeBackend captures every Backend call as a human-readable log so tests
// can assert sequence + arguments without spinning a WebSocket.
type fakeBackend struct {
	calls []string
}

func (f *fakeBackend) log(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeBackend) trace() string { return strings.Join(f.calls, "\n") }

func (f *fakeBackend) MouseMove(_ context.Context, x, y float64) error {
	f.log("MouseMove(%g,%g)", x, y)
	return nil
}

func (f *fakeBackend) MouseDown(_ context.Context, x, y float64, b MouseButton) error {
	f.log("MouseDown(%g,%g,%d)", x, y, int(b))
	return nil
}

func (f *fakeBackend) MouseUp(_ context.Context, x, y float64, b MouseButton) error {
	f.log("MouseUp(%g,%g,%d)", x, y, int(b))
	return nil
}

func (f *fakeBackend) MouseScroll(_ context.Context, x, y, dx, dy float64) error {
	f.log("MouseScroll(%g,%g,%g,%g)", x, y, dx, dy)
	return nil
}

func (f *fakeBackend) KeyChar(_ context.Context, ch rune) error {
	f.log("KeyChar(%q)", string(ch))
	return nil
}

func (f *fakeBackend) InsertText(_ context.Context, t string) error {
	f.log("InsertText(%q)", t)
	return nil
}

func (f *fakeBackend) KeyCombo(_ context.Context, vk uint16, m Modifier) error {
	f.log("KeyCombo(vk=0x%X,mods=%d)", vk, int(m))
	return nil
}

// SetWindow is unused by this fake — the frame lives in the real native backend.
func (*fakeBackend) SetWindow(Window) {}
