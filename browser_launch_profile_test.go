package firefoxkit

import (
	"os"
	"testing"
)

// TestBrowserClose_CallsProfileCleanup locks in the H2 fix: Browser.Close must
// remove the temp profile dir on the success path (previously cleanup only ran
// on Launch error branches, leaking one profile dir per Launch+Close).
func TestBrowserClose_CallsProfileCleanup(t *testing.T) {
	called := false
	b := &Browser{cleanupProfile: func() { called = true }}
	b.initBaseCtx()

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !called {
		t.Fatal("Close must invoke cleanupProfile for temp profiles")
	}
}

// TestBrowserClose_NilCleanupProfile: Connect-path browsers have no profile to
// clean — Close must not panic.
func TestBrowserClose_NilCleanupProfile(t *testing.T) {
	b := &Browser{}
	b.initBaseCtx()
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestEnsureProfileDir_TempDir: without a user dir, a temp profile is created
// and its cleanup removes it.
func TestEnsureProfileDir_TempDir(t *testing.T) {
	cfg := defaultConfig()
	dir, cleanup, err := ensureProfileDir(cfg)
	if err != nil {
		t.Fatalf("ensureProfileDir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp profile dir missing: %v", err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temp profile dir still exists after cleanup: %v", err)
	}
}

// TestEnsureProfileDir_UserDirNotRemoved: a user-supplied profile dir must
// survive cleanup — only firefoxkit-created temp dirs may be deleted.
func TestEnsureProfileDir_UserDirNotRemoved(t *testing.T) {
	userDir := t.TempDir()
	cfg := defaultConfig()
	cfg.userDataDir = userDir

	dir, cleanup, err := ensureProfileDir(cfg)
	if err != nil {
		t.Fatalf("ensureProfileDir: %v", err)
	}
	if dir != userDir {
		t.Fatalf("dir = %q, want user dir %q", dir, userDir)
	}
	cleanup()
	if _, err := os.Stat(userDir); err != nil {
		t.Fatalf("user profile dir must survive cleanup: %v", err)
	}
}
