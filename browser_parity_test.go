package firefoxkit

import "testing"

func TestBrowser_PID_ZeroWhenNoProc(t *testing.T) {
	b := newTestBrowser()
	var _ uint32 = b.PID() // compile-asserts the uint32 return type (chromekit parity)
	if b.PID() != 0 {
		t.Fatalf("PID with nil proc: want 0, got %d", b.PID())
	}
}

func TestBrowser_SetHWND_Override(t *testing.T) {
	b := newTestBrowser()
	b.SetHWND(0xDEAD)
	if b.hwndOverride != 0xDEAD {
		t.Fatalf("SetHWND: want override 0xDEAD, got %#x", b.hwndOverride)
	}
}

func TestBrowser_SwitchPageByIndex_OutOfRange(t *testing.T) {
	b := newTestBrowser()
	b.pages = []*Page{{browser: b, contextID: "a"}}
	b.current = b.pages[0]
	if err := b.SwitchPageByIndex(5); err != ErrPageNotFound {
		t.Fatalf("out-of-range index: want ErrPageNotFound, got %v", err)
	}
}

func TestBrowser_GoBack_NoCurrent(t *testing.T) {
	b := newTestBrowser()
	if err := b.GoBack(); err != ErrPageNotFound {
		t.Fatalf("GoBack with no current page: want ErrPageNotFound, got %v", err)
	}
}
