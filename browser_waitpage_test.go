package firefoxkit

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForNewPage_NilClient(t *testing.T) {
	b := &Browser{} // client nil
	_, err := b.WaitForNewPage("accounts.google.com", 50*time.Millisecond)
	if !errors.Is(err, ErrBiDiSessionFailed) {
		t.Fatalf("want ErrBiDiSessionFailed with nil client, got %v", err)
	}
}

func TestKnownContextIDs_Snapshot(t *testing.T) {
	b := &Browser{pages: []*Page{{contextID: "a"}, {contextID: "b"}}}
	got := b.knownContextIDs()
	if !got["a"] || !got["b"] || len(got) != 2 {
		t.Fatalf("knownContextIDs = %v, want {a,b}", got)
	}
}

func TestBindNewContext_DedupsExisting(t *testing.T) {
	b := &Browser{pages: []*Page{{contextID: "a"}}}
	if p := b.bindNewContext("a", "u"); p != nil {
		t.Fatalf("bindNewContext(existing) = %v, want nil (deduped)", p)
	}
	if p := b.bindNewContext("c", "https://accounts.google.com/x"); p == nil {
		t.Fatal("bindNewContext(new) = nil, want *Page")
	}
	if len(b.pages) != 2 {
		t.Fatalf("pages len = %d, want 2", len(b.pages))
	}
}
