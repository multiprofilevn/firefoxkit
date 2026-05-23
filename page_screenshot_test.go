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
	if ConsoleDebug != "debug" || ConsoleInfo != "info" ||
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
