package firefoxkit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestReencodeAsJPEG_RoundTrip(t *testing.T) {
	// Build a tiny 2x2 PNG in-memory.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(1, 0, color.RGBA{0, 255, 0, 255})
	img.Set(0, 1, color.RGBA{0, 0, 255, 255})
	img.Set(1, 1, color.RGBA{255, 255, 255, 255})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}

	jpg, err := reencodeAsJPEG(pngBuf.Bytes(), 70)
	if err != nil {
		t.Fatalf("reencode: %v", err)
	}
	// JPEG starts with FF D8 FF.
	if len(jpg) < 3 || jpg[0] != 0xFF || jpg[1] != 0xD8 || jpg[2] != 0xFF {
		t.Errorf("not a JPEG: %x", jpg[:min(8, len(jpg))])
	}
}

func TestReencodeAsJPEG_BadPNG(t *testing.T) {
	if _, err := reencodeAsJPEG([]byte("not a png"), 50); err == nil {
		t.Fatal("expected error on bad PNG")
	}
}

func TestDialogTypeConstants(t *testing.T) {
	if DialogAlert != "alert" || DialogConfirm != "confirm" ||
		DialogPrompt != "prompt" || DialogBeforeUnload != "beforeunload" {
		t.Errorf("dialog type constants drifted")
	}
}

func TestConsoleLevelConstants(t *testing.T) {
	if ConsoleLog != "log" || ConsoleDebug != "debug" || ConsoleInfo != "info" ||
		ConsoleWarn != "warn" || ConsoleError != "error" {
		t.Errorf("console level constants drifted")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestPage_SetSelectValue_SignatureCheck verifies SetSelectValue compiles with
// the correct chromekit-parity signature: (selector, value string) error.
// We use a compile-time assignment instead of calling into the nil bidi client.
func TestPage_SetSelectValue_SignatureCheck(t *testing.T) {
	// Confirm the method set exists with the right signature at compile time.
	var _ func(string, string) error = (&Page{}).SetSelectValue
}

// TestPermission_Constants verifies the Permission constants match the
// chromekit-canonical W3C strings exactly.
func TestPermission_Constants(t *testing.T) {
	cases := []struct {
		got  Permission
		want string
	}{
		{PermissionGeolocation, "geolocation"},
		{PermissionCamera, "camera"},
		{PermissionMicrophone, "microphone"},
		{PermissionNotifications, "notifications"},
		{PermissionClipboardRead, "clipboard-read"},
		{PermissionClipboardWrite, "clipboard-write"},
		{PermissionMIDI, "midi"},
		{PermissionMIDISysex, "midi-sysex"},
		{PermissionBackgroundSync, "background-sync"},
		{PermissionPersistentStorage, "persistent-storage"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Permission %q: got %q, want %q", c.want, c.got, c.want)
		}
	}
}
