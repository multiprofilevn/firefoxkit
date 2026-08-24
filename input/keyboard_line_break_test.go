package input

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// A line break must reach the backend as a VK_RETURN combo, never as a
// printable character. KeyChar('\n') is wrong on both backends: the remote
// backend sends it as a printable key value, which an editor that consumes
// Enter in its own keydown handler (Draft.js and friends) turns into TWO line
// breaks; native injects U+000A as a Unicode scan code, which the browser
// drops, losing the break. These tests pin the Enter press so neither
// regression can come back unnoticed.

// enterTrace is the call sequence expected for the input "A<break>B".
func enterTrace() string {
	return fmt.Sprintf("KeyChar(%q)\nKeyCombo(vk=0x%X,mods=%d)\nKeyChar(%q)",
		"A", VKReturn, 0, "B")
}

func TestTypeSendsLineBreakAsEnter(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"LF", "A\nB"},
		{"CRLF collapses to one Enter", "A\r\nB"},
		{"lone CR", "A\rB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeBackend{}
			k := &Keyboard{backend: f, MinDelay: time.Nanosecond, MaxDelay: time.Nanosecond}
			if err := k.Type(context.Background(), tc.in); err != nil {
				t.Fatalf("Type: %v", err)
			}
			if got := f.trace(); got != enterTrace() {
				t.Fatalf("trace:\n%s\nwant:\n%s", got, enterTrace())
			}
		})
	}
}

func TestTypeHumanSendsLineBreakAsEnter(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"LF", "A\nB"},
		{"CRLF collapses to one Enter", "A\r\nB"},
		{"lone CR", "A\rB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeBackend{}
			k := &Keyboard{backend: f}
			// MistakeRate 0: a typo would add backspace calls and this test is
			// about the break, not the typo simulation.
			cfg := HumanTypeConfig{
				MinDelay:    time.Nanosecond,
				MaxDelay:    time.Nanosecond,
				MistakeRate: 0,
			}
			if err := k.TypeHuman(context.Background(), tc.in, cfg); err != nil {
				t.Fatalf("TypeHuman: %v", err)
			}
			if got := f.trace(); got != enterTrace() {
				t.Fatalf("trace:\n%s\nwant:\n%s", got, enterTrace())
			}
		})
	}
}

func TestPressSendsLineBreakAsEnter(t *testing.T) {
	want := fmt.Sprintf("KeyCombo(vk=0x%X,mods=%d)", VKReturn, 0)
	for _, ch := range []rune{'\n', '\r'} {
		f := &fakeBackend{}
		k := &Keyboard{backend: f}
		if err := k.Press(context.Background(), ch); err != nil {
			t.Fatalf("Press(%q): %v", ch, err)
		}
		if got := f.trace(); got != want {
			t.Fatalf("Press(%q) trace = %q, want %q", ch, got, want)
		}
	}
}

func TestNormalizeLineBreaks(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"no breaks", "no breaks"},
		{"a\nb", "a\nb"},
		{"a\r\nb", "a\nb"},
		{"a\rb", "a\nb"},
		{"a\r\n\r\nb", "a\n\nb"},
	} {
		if got := normalizeLineBreaks(tc.in); got != tc.want {
			t.Fatalf("normalizeLineBreaks(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
