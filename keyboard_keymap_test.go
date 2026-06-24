package firefoxkit

import (
	"testing"
)

// TestResolveKey_NamedKeys verifies that every key name chromekit supports
// resolves to the correct entry and matches the expected w3cKey value.
func TestResolveKey_NamedKeys(t *testing.T) {
	cases := []struct {
		name    string
		w3cKey  string // expected W3C BiDi value
		wantVK  uint16 // non-zero means VK must match
		wantPrintable bool
	}{
		{"Enter",      "", 0x0D, false},
		{"Tab",        "", 0x09, false},
		{"Escape",     "", 0x1B, false},
		{"Esc",        "", 0x1B, false}, // alias
		{"Backspace",  "", 0x08, false},
		{"Delete",     "", 0x2E, false},
		{"ArrowUp",    "", 0x26, false},
		{"ArrowDown",  "", 0x28, false},
		{"ArrowLeft",  "", 0x25, false},
		{"ArrowRight", "", 0x27, false},
		{"Home",       "", 0x24, false},
		{"End",        "", 0x23, false},
		{"PageUp",     "", 0x21, false},
		{"PageDown",   "", 0x22, false},
		{"Space",      " ",      0x20, true},
		{"Control",   "", 0x11, false},
		{"Shift",     "", 0x10, false},
		{"Alt",       "", 0x12, false},
		{"Meta",      "", 0x5B, false},
		{"F1",  "", 0x70, false},
		{"F2",  "", 0x71, false},
		{"F3",  "", 0x72, false},
		{"F4",  "", 0x73, false},
		{"F5",  "", 0x74, false},
		{"F6",  "", 0x75, false},
		{"F7",  "", 0x76, false},
		{"F8",  "", 0x77, false},
		{"F9",  "", 0x78, false},
		{"F10", "", 0x79, false},
		{"F11", "", 0x7A, false},
		{"F12", "", 0x7B, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			entry, ok := resolveKey(tc.name)
			if !ok {
				t.Fatalf("resolveKey(%q) returned not-found", tc.name)
			}
			if entry.w3cKey != tc.w3cKey {
				t.Errorf("w3cKey = %q, want %q", entry.w3cKey, tc.w3cKey)
			}
			if tc.wantVK != 0 && entry.vk != tc.wantVK {
				t.Errorf("vk = 0x%X, want 0x%X", entry.vk, tc.wantVK)
			}
			if entry.isPrintable != tc.wantPrintable {
				t.Errorf("isPrintable = %v, want %v", entry.isPrintable, tc.wantPrintable)
			}
		})
	}
}

// TestResolveKey_PrintableChar verifies that single printable characters
// resolve via the fallback path (not the named-key map).
func TestResolveKey_PrintableChar(t *testing.T) {
	cases := []string{"a", "z", "A", "1", "?", "@"}
	for _, ch := range cases {
		entry, ok := resolveKey(ch)
		if !ok {
			t.Errorf("resolveKey(%q) not found", ch)
			continue
		}
		if entry.w3cKey != ch {
			t.Errorf("resolveKey(%q).w3cKey = %q, want %q", ch, entry.w3cKey, ch)
		}
		if !entry.isPrintable {
			t.Errorf("resolveKey(%q).isPrintable = false, want true", ch)
		}
	}
}

// TestResolveKey_Unknown confirms that multi-char strings with no named
// mapping return (zero, false).
func TestResolveKey_Unknown(t *testing.T) {
	for _, k := range []string{"NotAKey", "enter", "ENTER", ""} {
		if _, ok := resolveKey(k); ok {
			t.Errorf("resolveKey(%q) should return not-found", k)
		}
	}
}
