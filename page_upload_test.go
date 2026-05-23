package firefoxkit

import "testing"

func TestGuessMime(t *testing.T) {
	cases := []struct {
		path string
		mime string
	}{
		{"foo.png", "image/png"},
		{"foo.JPG", "image/jpeg"}, // case-insensitive — guessMime lower-cases the ext
		{"foo.jpg", "image/jpeg"},
		{"foo.pdf", "application/pdf"},
		{"foo.zip", "application/zip"},
		{"foo.bin", "application/octet-stream"},
		{"no_extension", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := guessMime(c.path); got != c.mime {
			t.Errorf("guessMime(%q)=%q, want %q", c.path, got, c.mime)
		}
	}
}

func TestSortedHeaderKeys(t *testing.T) {
	m := map[string]string{"X-B": "2", "X-A": "1", "X-C": "3"}
	got := sortedHeaderKeys(m)
	want := []string{"X-A", "X-B", "X-C"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}
